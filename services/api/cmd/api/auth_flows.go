package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type emailCodeRequest struct {
	Email    string `json:"email"`
	Code     string `json:"code"`
	Password string `json:"password"`
}
type mfaCodeRequest struct {
	ChallengeID string `json:"challenge_id"`
	Code        string `json:"code"`
}

func authCodeHash(purpose, code string) []byte {
	key := []byte(strings.TrimSpace(os.Getenv("JWT_SECRET")))
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(purpose + ":" + code))
	return mac.Sum(nil)
}
func encryptAuthSecret(plain string) (string, error) {
	key := []byte(strings.TrimSpace(os.Getenv("AUTH_ENCRYPTION_KEY")))
	if len(key) < 32 {
		return "", fmt.Errorf("AUTH_ENCRYPTION_KEY must contain at least 32 characters")
	}
	sum := sha256.Sum256(key)
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nil, nonce, []byte(plain), []byte("mekuin-auth-mfa-v1"))
	return "enc:v1:" + base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(append(nonce, sealed...)), nil
}
func decryptAuthSecret(value string) (string, error) {
	if !strings.HasPrefix(value, "enc:v1:") {
		return "", fmt.Errorf("MFA secret is not encrypted")
	}
	key := []byte(strings.TrimSpace(os.Getenv("AUTH_ENCRYPTION_KEY")))
	if len(key) < 32 {
		return "", fmt.Errorf("AUTH_ENCRYPTION_KEY must contain at least 32 characters")
	}
	raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.TrimPrefix(value, "enc:v1:"))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(key)
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("invalid encrypted MFA secret")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], []byte("mekuin-auth-mfa-v1"))
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
func randomSixDigitCode() (string, error) {
	b := make([]byte, 4)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	n := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	return fmt.Sprintf("%06d", n%1000000), nil
}
func sendAuthEmail(to, subject, body string) error {
	host := strings.TrimSpace(os.Getenv("SMTP_HOST"))
	from := strings.TrimSpace(os.Getenv("SMTP_FROM"))
	port := strings.TrimSpace(os.Getenv("SMTP_PORT"))
	if host == "" || from == "" {
		return fmt.Errorf("email_delivery_not_configured")
	}
	if port == "" {
		port = "587"
	}
	addr := host + ":" + port
	var auth smtp.Auth
	user := os.Getenv("SMTP_USERNAME")
	if user == "" {
		user = os.Getenv("SMTP_USER")
	}
	if user != "" {
		auth = smtp.PlainAuth("", user, os.Getenv("SMTP_PASSWORD"), host)
	}
	msg := []byte("From: " + from + "\r\nTo: " + to + "\r\nSubject: " + subject + "\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + body + "\r\n")
	return smtp.SendMail(addr, auth, from, []string{to}, msg)
}
func (s *server) issueEmailCode(userID, email, purpose, subject, body string) error {
	var recent int
	if err := s.db.QueryRow(context.Background(), `SELECT count(*) FROM auth_email_challenges WHERE user_id=$1 AND purpose=$2 AND created_at>now()-interval '60 seconds'`, userID, purpose).Scan(&recent); err != nil {
		return err
	}
	if recent > 0 {
		return fmt.Errorf("verification_email_rate_limited")
	}
	if len(strings.TrimSpace(os.Getenv("JWT_SECRET"))) < 32 {
		return fmt.Errorf("JWT_SECRET must contain at least 32 characters")
	}
	code, err := randomSixDigitCode()
	if err != nil {
		return err
	}
	digest := authCodeHash(purpose, code)
	_, err = s.db.Exec(context.Background(), `INSERT INTO auth_email_challenges(user_id,purpose,code_hash,expires_at) VALUES($1,$2,$3,now()+interval '15 minutes')`, userID, purpose, digest)
	if err != nil {
		return err
	}
	return sendAuthEmail(email, subject, fmt.Sprintf(body, code))
}
func (s *server) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var in emailCodeRequest
	if !decode(w, r, &in) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	code := strings.TrimSpace(in.Code)
	if email == "" || len(code) != 6 {
		jsonOut(w, 400, map[string]string{"error": "invalid_verification_code"})
		return
	}
	var uid string
	err := s.db.QueryRow(r.Context(), `SELECT user_id FROM lookup_auth_user($1)`, email).Scan(&uid)
	if err != nil {
		jsonOut(w, 400, map[string]string{"error": "invalid_verification_code"})
		return
	}
	var id string
	var hash []byte
	var attempts int
	err = s.db.QueryRow(r.Context(), `SELECT id,code_hash,attempts FROM auth_email_challenges WHERE user_id=$1 AND purpose='verify_email' AND consumed_at IS NULL AND expires_at>now() ORDER BY created_at DESC LIMIT 1`, uid).Scan(&id, &hash, &attempts)
	if err != nil || attempts >= 10 || !hmac.Equal(hash, authCodeHash("verify_email", code)) {
		if id != "" {
			_, _ = s.db.Exec(r.Context(), `UPDATE auth_email_challenges SET attempts=attempts+1 WHERE id=$1 AND attempts<10`, id)
		}
		jsonOut(w, 400, map[string]string{"error": "invalid_or_expired_verification_code"})
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		jsonOut(w, 503, map[string]string{"error": "verification_unavailable"})
		return
	}
	defer tx.Rollback(r.Context())
	_, err = tx.Exec(r.Context(), `SELECT auth_set_email_verified($1)`, id)
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		jsonOut(w, 503, map[string]string{"error": "verification_unavailable"})
		return
	}
	if err = s.createSession(w, r, uid); err != nil {
		jsonOut(w, 500, map[string]string{"error": "could_not_create_session"})
		return
	}
	jsonOut(w, 200, map[string]string{"status": "verified"})
}
func (s *server) resendVerification(w http.ResponseWriter, r *http.Request) {
	var in emailCodeRequest
	if !decode(w, r, &in) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	var uid string
	if err := s.db.QueryRow(r.Context(), `SELECT user_id FROM lookup_auth_user($1) WHERE email_verified_at IS NULL`, email).Scan(&uid); err == nil {
		if e := s.issueEmailCode(uid, email, "verify_email", "Verify your MEKU-In email", "Your MEKU-In verification code is %s. It expires in 15 minutes."); e != nil {
			s.log.Warn("verification email could not be sent", "error", e)
		}
	}
	jsonOut(w, 202, map[string]string{"status": "if_verification_is_required_an_email_will_be_sent"})
}
func (s *server) requestPasswordReset(w http.ResponseWriter, r *http.Request) {
	var in emailCodeRequest
	if !decode(w, r, &in) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	var uid string
	if err := s.db.QueryRow(r.Context(), `SELECT user_id FROM lookup_auth_user($1) WHERE email_verified_at IS NOT NULL AND account_status='active'`, email).Scan(&uid); err == nil {
		if e := s.issueEmailCode(uid, email, "password_reset", "Reset your MEKU-In password", "Your MEKU-In password reset code is %s. It expires in 15 minutes."); e != nil {
			s.log.Warn("password reset email could not be sent", "error", e)
		}
	}
	jsonOut(w, 202, map[string]string{"status": "if_the_account_exists_a_reset_code_will_be_sent"})
}
func (s *server) resetPassword(w http.ResponseWriter, r *http.Request) {
	var in emailCodeRequest
	if !decode(w, r, &in) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if len(in.Code) != 6 || len(in.Password) < 12 || len(in.Password) > 72 {
		jsonOut(w, 400, map[string]string{"error": "invalid_reset_request"})
		return
	}
	var uid, id string
	var hash []byte
	var attempts int
	err := s.db.QueryRow(r.Context(), `SELECT u.user_id,c.id,c.code_hash,c.attempts FROM lookup_auth_user($1) u JOIN LATERAL (SELECT * FROM auth_email_challenges WHERE user_id=u.user_id AND purpose='password_reset' AND consumed_at IS NULL AND expires_at>now() ORDER BY created_at DESC LIMIT 1)c ON true WHERE u.email_verified_at IS NOT NULL AND u.account_status='active'`, email).Scan(&uid, &id, &hash, &attempts)
	if err != nil || attempts >= 10 || !hmac.Equal(hash, authCodeHash("password_reset", in.Code)) {
		if id != "" {
			_, _ = s.db.Exec(r.Context(), `UPDATE auth_email_challenges SET attempts=attempts+1 WHERE id=$1 AND attempts<10`, id)
		}
		jsonOut(w, 400, map[string]string{"error": "invalid_or_expired_reset_code"})
		return
	}
	pw, err := bcryptHash(in.Password)
	if err != nil {
		jsonOut(w, 500, map[string]string{"error": "password_update_failed"})
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		jsonOut(w, 503, map[string]string{"error": "password_update_unavailable"})
		return
	}
	defer tx.Rollback(r.Context())
	_, err = tx.Exec(r.Context(), `SELECT auth_set_password($1,$2)`, id, pw)
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		jsonOut(w, 503, map[string]string{"error": "password_update_unavailable"})
		return
	}
	clearCookie(w, s.cookieSecure)
	jsonOut(w, 200, map[string]string{"status": "password_reset"})
}
func bcryptHash(password string) (string, error) {
	b, e := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), e
}
func (s *server) mfaSetup(w http.ResponseWriter, r *http.Request) {
	uid := r.Context().Value(userIDKey).(string)
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		jsonOut(w, 503, map[string]string{"error": "mfa_setup_unavailable"})
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `SELECT set_config('app.user_id',$1,true)`, uid); err != nil {
		jsonOut(w, 503, map[string]string{"error": "mfa_setup_unavailable"})
		return
	}
	var email string
	var alreadyEnabled bool
	if err = tx.QueryRow(r.Context(), `SELECT email,mfa_enabled FROM lookup_auth_user_by_id($1)`, uid).Scan(&email, &alreadyEnabled); err != nil {
		jsonOut(w, 503, map[string]string{"error": "mfa_setup_unavailable"})
		return
	}
	if alreadyEnabled {
		jsonOut(w, 409, map[string]string{"error": "mfa_already_enabled"})
		return
	}
	raw := make([]byte, 20)
	if _, err = rand.Read(raw); err != nil {
		jsonOut(w, 500, map[string]string{"error": "mfa_setup_failed"})
		return
	}
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
	sealed, err := encryptAuthSecret(secret)
	if err != nil {
		jsonOut(w, 503, map[string]string{"error": "mfa_encryption_not_configured"})
		return
	}
	if _, err = tx.Exec(r.Context(), `SELECT auth_set_mfa($1,$2,false)`, uid, sealed); err != nil {
		jsonOut(w, 500, map[string]string{"error": "mfa_setup_failed"})
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		jsonOut(w, 503, map[string]string{"error": "mfa_setup_failed"})
		return
	}
	issuer := url.QueryEscape("MEKU-In")
	label := url.QueryEscape(email)
	uri := "otpauth://totp/" + issuer + ":" + label + "?secret=" + secret + "&issuer=" + issuer + "&algorithm=SHA1&digits=6&period=30"
	jsonOut(w, 200, map[string]string{"secret": secret, "otpauth_url": uri, "status": "pending_confirmation"})
}
func totpValid(secret, code string, now time.Time) bool {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil || len(code) != 6 {
		return false
	}
	step := now.Unix() / 30
	for d := int64(-1); d <= 1; d++ {
		var msg [8]byte
		v := uint64(step + d)
		for i := 7; i >= 0; i-- {
			msg[i] = byte(v)
			v >>= 8
		}
		mac := hmac.New(sha1.New, key)
		mac.Write(msg[:])
		sum := mac.Sum(nil)
		off := sum[len(sum)-1] & 0x0f
		n := (uint32(sum[off])&0x7f)<<24 | uint32(sum[off+1])<<16 | uint32(sum[off+2])<<8 | uint32(sum[off+3])
		if fmt.Sprintf("%06d", n%1000000) == code {
			return true
		}
	}
	return false
}
func (s *server) mfaEnable(w http.ResponseWriter, r *http.Request) {
	uid := r.Context().Value(userIDKey).(string)
	var in mfaCodeRequest
	if !decode(w, r, &in) {
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		jsonOut(w, 503, map[string]string{"error": "mfa_enable_unavailable"})
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `SELECT set_config('app.user_id',$1,true)`, uid); err != nil {
		jsonOut(w, 503, map[string]string{"error": "mfa_enable_unavailable"})
		return
	}
	var sealed string
	if err = tx.QueryRow(r.Context(), `SELECT mfa_secret FROM lookup_auth_user_by_id($1)`, uid).Scan(&sealed); err != nil {
		jsonOut(w, 400, map[string]string{"error": "invalid_mfa_code"})
		return
	}
	secret, err := decryptAuthSecret(sealed)
	if err != nil || !totpValid(secret, in.Code, time.Now()) {
		jsonOut(w, 400, map[string]string{"error": "invalid_mfa_code"})
		return
	}
	if _, err = tx.Exec(r.Context(), `SELECT auth_set_mfa($1,$2,true)`, uid, sealed); err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		jsonOut(w, 500, map[string]string{"error": "mfa_enable_failed"})
		return
	}
	jsonOut(w, 200, map[string]string{"status": "enabled"})
}
func (s *server) mfaDisable(w http.ResponseWriter, r *http.Request) {
	uid := r.Context().Value(userIDKey).(string)
	var in mfaCodeRequest
	if !decode(w, r, &in) {
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		jsonOut(w, 503, map[string]string{"error": "mfa_disable_unavailable"})
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `SELECT set_config('app.user_id',$1,true)`, uid); err != nil {
		jsonOut(w, 503, map[string]string{"error": "mfa_disable_unavailable"})
		return
	}
	var sealed string
	if err = tx.QueryRow(r.Context(), `SELECT mfa_secret FROM lookup_auth_user_by_id($1) WHERE mfa_enabled=true`, uid).Scan(&sealed); err != nil {
		jsonOut(w, 400, map[string]string{"error": "invalid_mfa_code"})
		return
	}
	secret, err := decryptAuthSecret(sealed)
	if err != nil || !totpValid(secret, in.Code, time.Now()) {
		jsonOut(w, 400, map[string]string{"error": "invalid_mfa_code"})
		return
	}
	if _, err = tx.Exec(r.Context(), `SELECT auth_set_mfa($1,NULL,false)`, uid); err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		jsonOut(w, 500, map[string]string{"error": "mfa_disable_failed"})
		return
	}
	jsonOut(w, 200, map[string]string{"status": "disabled"})
}

func (s *server) verifyLoginMFA(w http.ResponseWriter, r *http.Request) {
	var in mfaCodeRequest
	if !decode(w, r, &in) {
		return
	}
	var uid string
	var secret string
	var challengeID string
	err := s.db.QueryRow(r.Context(), `SELECT challenge_id,user_id,mfa_secret FROM lookup_mfa_challenge($1) WHERE account_status='active' AND email_verified_at IS NOT NULL AND mfa_enabled=true`, in.ChallengeID).Scan(&challengeID, &uid, &secret)
	if err == nil {
		secret, err = decryptAuthSecret(secret)
	}
	if err != nil || !totpValid(secret, in.Code, time.Now()) {
		if challengeID != "" {
			_, _ = s.db.Exec(r.Context(), `UPDATE auth_mfa_challenges SET attempts=attempts+1 WHERE id=$1`, challengeID)
		}
		jsonOut(w, 401, map[string]string{"error": "invalid_or_expired_mfa_challenge"})
		return
	}
	var consumedID string
	err = s.db.QueryRow(r.Context(), `UPDATE auth_mfa_challenges SET consumed_at=now() WHERE id=$1 AND consumed_at IS NULL AND expires_at>now() AND attempts<10 RETURNING id`, challengeID).Scan(&consumedID)
	if err != nil || consumedID == "" {
		jsonOut(w, 401, map[string]string{"error": "invalid_or_expired_mfa_challenge"})
		return
	}
	if err = s.createSession(w, r, uid); err != nil {
		jsonOut(w, 500, map[string]string{"error": "could_not_create_session"})
		return
	}
	jsonOut(w, 200, map[string]string{"status": "authenticated"})
}

// Keep JSON error responses stable for clients while avoiding disclosure of whether reset emails exist.

type googleTokenInfo struct {
	Audience      string `json:"aud"`
	Issuer        string `json:"iss"`
	Subject       string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified string `json:"email_verified"`
}

func (s *server) googleSignIn(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Credential string `json:"credential"`
	}
	if !decode(w, r, &in) {
		return
	}
	clientID := strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID"))
	if clientID == "" {
		jsonOut(w, 503, map[string]string{"error": "google_sign_in_not_configured"})
		return
	}
	if in.Credential == "" || len(in.Credential) > 8192 {
		jsonOut(w, 400, map[string]string{"error": "invalid_google_credential"})
		return
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://oauth2.googleapis.com/tokeninfo?id_token=" + url.QueryEscape(in.Credential))
	if err != nil {
		jsonOut(w, 503, map[string]string{"error": "google_identity_verification_unavailable"})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		jsonOut(w, 401, map[string]string{"error": "invalid_google_credential"})
		return
	}
	var info googleTokenInfo
	if err = json.NewDecoder(resp.Body).Decode(&info); err != nil || info.Audience != clientID || (info.Issuer != "accounts.google.com" && info.Issuer != "https://accounts.google.com") || info.Subject == "" || !strings.EqualFold(info.EmailVerified, "true") {
		jsonOut(w, 401, map[string]string{"error": "invalid_google_identity"})
		return
	}
	var uid, status string
	var verified *time.Time
	var mfa bool
	err = s.db.QueryRow(r.Context(), `SELECT user_id,account_status,email_verified_at,mfa_enabled FROM lookup_auth_user($1)`, strings.ToLower(info.Email)).Scan(&uid, &status, &verified, &mfa)
	if err != nil {
		jsonOut(w, 404, map[string]string{"error": "google_account_not_registered", "next": "register_with_email_then_verify_to_create_a_workspace"})
		return
	}
	if status != "active" {
		jsonOut(w, 403, map[string]string{"error": "account_suspended"})
		return
	}
	if verified == nil {
		jsonOut(w, 403, map[string]string{"error": "email_verification_required"})
		return
	}
	if mfa {
		var challenge string
		err = s.db.QueryRow(r.Context(), `INSERT INTO auth_mfa_challenges(user_id,code_hash,expires_at) VALUES($1,$2,now()+interval '5 minutes') RETURNING id`, uid, authCodeHash("mfa_challenge", uid+time.Now().UTC().Format(time.RFC3339Nano))).Scan(&challenge)
		if err != nil {
			jsonOut(w, 503, map[string]string{"error": "mfa_challenge_failed"})
			return
		}
		jsonOut(w, 200, map[string]string{"status": "mfa_required", "challenge_id": challenge})
		return
	}
	if err = s.createSession(w, r, uid); err != nil {
		jsonOut(w, 500, map[string]string{"error": "could_not_create_session"})
		return
	}
	jsonOut(w, 200, map[string]string{"status": "authenticated"})
}

func (s *server) mfaStatus(w http.ResponseWriter, r *http.Request) {
	uid := r.Context().Value(userIDKey).(string)
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		jsonOut(w, 503, map[string]string{"error": "mfa_status_unavailable"})
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `SELECT set_config('app.user_id',$1,true)`, uid); err != nil {
		jsonOut(w, 503, map[string]string{"error": "mfa_status_unavailable"})
		return
	}
	var enabled bool
	if err = tx.QueryRow(r.Context(), `SELECT mfa_enabled FROM lookup_auth_user_by_id($1)`, uid).Scan(&enabled); err != nil {
		jsonOut(w, 503, map[string]string{"error": "mfa_status_unavailable"})
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		jsonOut(w, 503, map[string]string{"error": "mfa_status_unavailable"})
		return
	}
	jsonOut(w, 200, map[string]bool{"enabled": enabled})
}
