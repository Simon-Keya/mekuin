package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidEmail(t *testing.T) {
	valid := []string{"owner@example.com", "a.b+pos@example.co.ke"}
	for _, value := range valid {
		if !validEmail(value) {
			t.Errorf("expected %q to be valid", value)
		}
	}
	invalid := []string{"", "owner", "owner@", "@example.com", "owner@example", "owner @example.com", "a@@example.com"}
	for _, value := range invalid {
		if validEmail(value) {
			t.Errorf("expected %q to be invalid", value)
		}
	}
}

func TestRegistrationPasswordByteBoundaries(t *testing.T) {
	base := registerRequest{FullName: "Test Owner", Email: "owner@example.com", BusinessName: "Test Bar", BranchName: "Main Branch"}
	base.Password = strings.Repeat("a", 12)
	if !validRegistration(base) {
		t.Fatal("12-byte password should be accepted")
	}
	base.Password = strings.Repeat("a", 72)
	if !validRegistration(base) {
		t.Fatal("72-byte password should be accepted")
	}
	base.Password = strings.Repeat("a", 73)
	if validRegistration(base) {
		t.Fatal("73-byte password must be rejected before bcrypt")
	}
}

func TestLoginPasswordByteBoundaries(t *testing.T) {
	if !validLogin("owner@example.com", strings.Repeat("x", 72)) {
		t.Fatal("72-byte password should be accepted")
	}
	if validLogin("owner@example.com", strings.Repeat("x", 73)) {
		t.Fatal("73-byte password must be rejected")
	}
	if validLogin("bad-email", "password") {
		t.Fatal("invalid email must be rejected")
	}
}

func TestDecodeRejectsTrailingJSON(t *testing.T) {
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"email":"a@example.com"} {"admin":true}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	var input loginRequest
	if decode(rr, req, &input) {
		t.Fatal("decoder must reject multiple JSON values")
	}
	if rr.Code != 400 {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestDecodeRequiresJSONContentType(t *testing.T) {
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"email":"a@example.com"}`))
	rr := httptest.NewRecorder()
	var input loginRequest
	if decode(rr, req, &input) {
		t.Fatal("decoder must reject missing content type")
	}
	if rr.Code != 415 {
		t.Fatalf("expected 415, got %d", rr.Code)
	}
}

func TestRequireSameOriginBlocksCrossSiteMutation(t *testing.T) {
	t.Setenv("WEB_ORIGIN", "https://app.example.test")
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := (&server{}).requireSameOrigin(next)

	cross := httptest.NewRequest("POST", "/api/v1/auth/logout", nil)
	cross.Header.Set("Origin", "https://attacker.example")
	crossRes := httptest.NewRecorder()
	h.ServeHTTP(crossRes, cross)
	if crossRes.Code != http.StatusForbidden {
		t.Fatalf("cross-site mutation got %d, want %d", crossRes.Code, http.StatusForbidden)
	}

	missing := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
	missingRes := httptest.NewRecorder()
	h.ServeHTTP(missingRes, missing)
	if missingRes.Code != http.StatusForbidden {
		t.Fatalf("missing Origin on cookie-issuing mutation got %d, want %d", missingRes.Code, http.StatusForbidden)
	}

	valid := httptest.NewRequest("POST", "/api/v1/tenant/select", nil)
	valid.Header.Set("Origin", "https://app.example.test")
	validRes := httptest.NewRecorder()
	h.ServeHTTP(validRes, valid)
	if validRes.Code != http.StatusNoContent {
		t.Fatalf("same-origin mutation got %d, want %d", validRes.Code, http.StatusNoContent)
	}
}

func TestDarajaPhoneValidation(t *testing.T) {
	for _, phone := range []string{"254712345678", "254799000001"} {
		if !validDarajaPhone(phone) {
			t.Errorf("expected valid phone %q", phone)
		}
	}
	for _, phone := range []string{"0712345678", "+254712345678", "25471234567", "25471234567x", "2547123456789"} {
		if validDarajaPhone(phone) {
			t.Errorf("expected invalid phone %q", phone)
		}
	}
}

func TestDarajaConfigurationRequiresHTTPSAndCredentials(t *testing.T) {
	t.Setenv("DARAJA_ENV", "sandbox")
	t.Setenv("DARAJA_CONSUMER_KEY", "key")
	t.Setenv("DARAJA_CONSUMER_SECRET", "secret")
	t.Setenv("DARAJA_SHORTCODE", "174379")
	t.Setenv("DARAJA_PASSKEY", "passkey")
	t.Setenv("DARAJA_CALLBACK_URL", "http://example.test/callback")
	if _, err := newDarajaClientFromEnv(); err == nil {
		t.Fatal("expected non-HTTPS callback URL to be rejected")
	}
	t.Setenv("DARAJA_CALLBACK_URL", "https://api.example.test/daraja/stk/callback")
	if _, err := newDarajaClientFromEnv(); err != nil {
		t.Fatalf("valid sandbox config rejected: %v", err)
	}
}
