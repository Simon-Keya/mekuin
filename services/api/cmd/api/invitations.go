package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"net/url"
	"os"
	"strings"
	"time"

	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"
)

type staffInvitationRequest struct {
	Email     string   `json:"email"`
	Role      string   `json:"role"`
	BranchIDs []string `json:"branch_ids"`
}

type acceptInvitationRequest struct {
	Token string `json:"token"`
}

type registerInvitationRequest struct {
	Token    string `json:"token"`
	FullName string `json:"full_name"`
	Password string `json:"password"`
}

func newInvitationToken() (string, []byte, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	return hex.EncodeToString(b), invitationTokenDigest(hex.EncodeToString(b)), nil
}

func invitationTokenDigest(token string) []byte {
	b, err := hex.DecodeString(token)
	if err != nil || len(b) != 32 {
		return nil
	}
	d := sha256.Sum256(b)
	return d[:]
}

func (s *server) createStaffInvitation(w http.ResponseWriter, r *http.Request) {
	tx, _, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in staffInvitationRequest
	if !decode(w, r, &in) {
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.Role = strings.TrimSpace(in.Role)
	roles := map[string]bool{"manager": true, "cashier": true, "bartender": true, "stock_controller": true}
	if !validEmail(in.Email) || !roles[in.Role] || len(in.BranchIDs) == 0 || len(in.BranchIDs) > 100 {
		jsonOut(w, 400, map[string]string{"error": "invalid_staff_invitation"})
		return
	}
	seen := map[string]bool{}
	for i, id := range in.BranchIDs {
		id = strings.ToLower(id)
		in.BranchIDs[i] = id
		if !validUUID(id) || seen[id] {
			jsonOut(w, 400, map[string]string{"error": "invalid_staff_branch_assignments"})
			return
		}
		seen[id] = true
		var exists bool
		if err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM branches WHERE business_id=$1 AND id=$2 AND is_active=true)`, tenantBusiness(r), id).Scan(&exists); err != nil {
			catalogError(w, err)
			return
		}
		if !exists {
			jsonOut(w, 422, map[string]string{"error": "branch_not_available_for_business"})
			return
		}
	}
	var existing bool
	if err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM memberships m WHERE m.business_id=$1 AND m.user_id=(SELECT user_id FROM lookup_staff_user($2)))`, tenantBusiness(r), in.Email).Scan(&existing); err != nil {
		catalogError(w, err)
		return
	}
	if existing {
		jsonOut(w, 409, map[string]string{"error": "user_already_has_business_membership"})
		return
	}
	if _, err := tx.Exec(r.Context(), `UPDATE staff_invitations SET revoked_at=now() WHERE business_id=$1 AND lower(invited_email)=lower($2) AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at<=now()`, tenantBusiness(r), in.Email); err != nil {
		catalogError(w, err)
		return
	}
	var pending bool
	if err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM staff_invitations WHERE business_id=$1 AND lower(invited_email)=lower($2) AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at>now())`, tenantBusiness(r), in.Email).Scan(&pending); err != nil {
		catalogError(w, err)
		return
	}
	if pending {
		jsonOut(w, 409, map[string]string{"error": "staff_invitation_already_pending"})
		return
	}
	token, digest, err := newInvitationToken()
	if err != nil {
		jsonOut(w, 500, map[string]string{"error": "invitation_generation_failed"})
		return
	}
	branchUUIDs := make([]pgtype.UUID, len(in.BranchIDs))
	for i, id := range in.BranchIDs {
		if err := branchUUIDs[i].Scan(id); err != nil {
			jsonOut(w, 400, map[string]string{"error": "invalid_staff_branch_assignments"})
			return
		}
	}
	var invitationID string
	err = tx.QueryRow(r.Context(), `INSERT INTO staff_invitations(business_id,invited_email,role,branch_ids,token_hash,invited_by,expires_at) VALUES($1,$2,$3,$4,$5,$6,now()+interval '72 hours') RETURNING id`, tenantBusiness(r), in.Email, in.Role, branchUUIDs, digest, r.Context().Value(userIDKey)).Scan(&invitationID)
	if err != nil {
		catalogError(w, err)
		return
	}
	var businessName string
	if err = tx.QueryRow(r.Context(), `SELECT name FROM businesses WHERE id=$1`, tenantBusiness(r)).Scan(&businessName); err != nil {
		catalogError(w, err)
		return
	}
	subject, body := staffInvitationEmailContent(businessName, token)
	if _, err = tx.Exec(r.Context(), `INSERT INTO email_outbox(business_id,recipient,subject,body) VALUES($1,$2,$3,$4)`, tenantBusiness(r), in.Email, subject, body); err != nil {
		catalogError(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary) VALUES($1,$2,'staff.invitation.created','staff_invitation',$3,'Staff invitation queued for email delivery')`, tenantBusiness(r), r.Context().Value(userIDKey), invitationID); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 201, map[string]any{"invitation_id": invitationID, "email": in.Email, "role": in.Role, "expires_in_hours": 72, "delivery": "queued"})
}

func staffInvitationEmailContent(businessName, token string) (string, string) {
	base := strings.TrimRight(env("APP_BASE_URL", env("WEB_ORIGIN", "http://localhost:3000")), "/")
	link := base + "/accept-invitation?token=" + url.QueryEscape(token)
	safeName := strings.ReplaceAll(strings.ReplaceAll(businessName, "\r", " "), "\n", " ")
	subject := "You are invited to join " + safeName
	body := "You have been invited to join " + safeName + " on MEKU-In.\r\n\r\nAccept your invitation within 72 hours: " + link + "\r\n\r\nIf you were not expecting this invitation, you can ignore this email.\r\n"
	return subject, body
}

func sendStaffInvitationEmail(to, businessName, token string) error {
	subject, body := staffInvitationEmailContent(businessName, token)
	return sendOutboxEmail(to, subject, body)
}

func sendOutboxEmail(to, subject, body string) error {
	host := strings.TrimSpace(os.Getenv("SMTP_HOST"))
	port := strings.TrimSpace(os.Getenv("SMTP_PORT"))
	from := strings.TrimSpace(os.Getenv("SMTP_FROM"))
	if host == "" || from == "" {
		return fmt.Errorf("SMTP_HOST and SMTP_FROM must be configured")
	}
	if port == "" {
		port = "587"
	}
	user, pass := os.Getenv("SMTP_USERNAME"), os.Getenv("SMTP_PASSWORD")
	msg := []byte("To: " + to + "\r\nFrom: " + from + "\r\nSubject: " + subject + "\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + body)
	conn, err := net.DialTimeout("tcp", host+":"+port, 10*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()
	if ok, _ := client.Extension("STARTTLS"); !ok {
		return fmt.Errorf("SMTP server does not advertise STARTTLS")
	}
	if err = client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
		return err
	}
	if user != "" {
		if ok, _ := client.Extension("AUTH"); !ok {
			return fmt.Errorf("SMTP server does not advertise AUTH")
		}
		if err = client.Auth(smtp.PlainAuth("", user, pass, host)); err != nil {
			return err
		}
	}
	if err = client.Mail(from); err != nil {
		return err
	}
	if err = client.Rcpt(to); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err = w.Write(msg); err != nil {
		_ = w.Close()
		return err
	}
	if err = w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func (s *server) registerStaffInvitee(w http.ResponseWriter, r *http.Request) {
	var in registerInvitationRequest
	if !decode(w, r, &in) {
		return
	}
	in.Token = strings.TrimSpace(in.Token)
	in.FullName = strings.TrimSpace(in.FullName)
	digest := invitationTokenDigest(in.Token)
	if digest == nil || len(in.FullName) < 2 || len(in.FullName) > 120 || len([]byte(in.Password)) < 12 || len([]byte(in.Password)) > 72 {
		jsonOut(w, 400, map[string]string{"error": "invalid_invited_account_details"})
		return
	}
	var invitationValid bool
	if err := s.db.QueryRow(r.Context(), `SELECT is_staff_invitation_valid($1)`, digest).Scan(&invitationValid); err != nil || !invitationValid {
		jsonOut(w, http.StatusGone, map[string]string{"error": "invitation_invalid_or_expired"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		jsonOut(w, 500, map[string]string{"error": "could_not_create_account"})
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		jsonOut(w, 503, map[string]string{"error": "invitation_registration_unavailable"})
		return
	}
	defer tx.Rollback(r.Context())
	var uid, email string
	err = tx.QueryRow(r.Context(), `SELECT user_id,email FROM register_staff_invitee($1,$2,$3)`, digest, in.FullName, string(hash)).Scan(&uid, &email)
	if err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "invitation_invalid_or_expired") {
			jsonOut(w, 410, map[string]string{"error": "invitation_invalid_or_expired"})
			return
		}
		if strings.Contains(msg, "invited_email_account_exists") {
			jsonOut(w, 409, map[string]string{"error": "invited_email_account_exists"})
			return
		}
		jsonOut(w, 503, map[string]string{"error": "invitation_registration_unavailable"})
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		jsonOut(w, 503, map[string]string{"error": "invitation_registration_unavailable"})
		return
	}
	if err = s.createSession(w, r, uid); err != nil {
		jsonOut(w, 503, map[string]string{"error": "account_created_sign_in_required"})
		return
	}
	jsonOut(w, 201, map[string]string{"user_id": uid, "email": email, "status": "registered"})
}

func (s *server) acceptStaffInvitation(w http.ResponseWriter, r *http.Request) {
	var in acceptInvitationRequest
	if !decode(w, r, &in) {
		return
	}
	in.Token = strings.TrimSpace(in.Token)
	digest := invitationTokenDigest(in.Token)
	if digest == nil {
		jsonOut(w, 400, map[string]string{"error": "invalid_invitation_token"})
		return
	}
	uid, _ := r.Context().Value(userIDKey).(string)
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		jsonOut(w, 503, map[string]string{"error": "invitation_acceptance_unavailable"})
		return
	}
	defer tx.Rollback(r.Context())
	var membershipID, businessID, role string
	err = tx.QueryRow(r.Context(), `SELECT membership_id,business_id,role FROM accept_staff_invitation($1,$2)`, digest, uid).Scan(&membershipID, &businessID, &role)
	if err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "invitation_invalid_or_expired") {
			jsonOut(w, 410, map[string]string{"error": "invitation_invalid_or_expired"})
			return
		}
		if strings.Contains(msg, "invitation_email_mismatch") {
			jsonOut(w, 403, map[string]string{"error": "invitation_email_mismatch"})
			return
		}
		if strings.Contains(msg, "invitation_branch_unavailable") {
			jsonOut(w, 409, map[string]string{"error": "invitation_branch_unavailable"})
			return
		}
		if strings.Contains(msg, "duplicate key") {
			jsonOut(w, 409, map[string]string{"error": "user_already_has_business_membership"})
			return
		}
		s.log.Error("staff invitation acceptance failed", "error", err)
		jsonOut(w, 503, map[string]string{"error": "invitation_acceptance_unavailable"})
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		jsonOut(w, 503, map[string]string{"error": "invitation_acceptance_unavailable"})
		return
	}
	jsonOut(w, 200, map[string]string{"membership_id": membershipID, "business_id": businessID, "role": role, "status": "active"})
}
