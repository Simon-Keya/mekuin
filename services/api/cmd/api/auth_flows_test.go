package main

import (
	"encoding/base32"
	"testing"
	"time"
)

func TestTOTPMatchesRFC6238SHA1Vector(t *testing.T) {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	if !totpValid(secret, "287082", time.Unix(59, 0)) {
		t.Fatal("expected RFC 6238 SHA1 6-digit code at Unix time 59 to validate")
	}
	if totpValid(secret, "287083", time.Unix(59, 0)) {
		t.Fatal("unexpected invalid TOTP code accepted")
	}
}

func TestAuthCodeHashSeparatesPurposes(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-with-at-least-thirty-two-characters")
	verify := authCodeHash("verify_email", "123456")
	reset := authCodeHash("password_reset", "123456")
	if string(verify) == string(reset) {
		t.Fatal("same code must hash differently for separate purposes")
	}
	if string(verify) != string(authCodeHash("verify_email", "123456")) {
		t.Fatal("same purpose and code must produce deterministic digest")
	}
}

func TestRandomSixDigitCodeFormat(t *testing.T) {
	for i := 0; i < 100; i++ {
		code, err := randomSixDigitCode()
		if err != nil {
			t.Fatal(err)
		}
		if len(code) != 6 {
			t.Fatalf("code %q is not six digits", code)
		}
		for _, r := range code {
			if r < '0' || r > '9' {
				t.Fatalf("code %q contains a non-digit", code)
			}
		}
	}
}

func TestMFASecretEncryptionRoundTripAndAuthentication(t *testing.T) {
	t.Setenv("AUTH_ENCRYPTION_KEY", "an-auth-encryption-key-at-least-32-characters-long")
	sealed, err := encryptAuthSecret("JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatal(err)
	}
	plain, err := decryptAuthSecret(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if plain != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("unexpected decrypted secret %q", plain)
	}
	t.Setenv("AUTH_ENCRYPTION_KEY", "a-different-encryption-key-at-least-32-characters")
	if _, err = decryptAuthSecret(sealed); err == nil {
		t.Fatal("ciphertext must not decrypt with a different key")
	}
}
