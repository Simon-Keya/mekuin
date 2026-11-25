package main

import (
	"net/http/httptest"
	"testing"
)

func TestIsActiveSuperAdmin(t *testing.T) {
	cases := []struct {
		role, status string
		want         bool
	}{
		{"super_admin", "active", true}, {"user", "active", false},
		{"super_admin", "suspended", false}, {"", "active", false},
	}
	for _, tc := range cases {
		if got := isActiveSuperAdmin(tc.role, tc.status); got != tc.want {
			t.Errorf("isActiveSuperAdmin(%q,%q)=%v want %v", tc.role, tc.status, got, tc.want)
		}
	}
}

func TestParsePlatformPage(t *testing.T) {
	cases := []struct {
		query         string
		limit, offset int
		ok            bool
	}{
		{"", 50, 0, true}, {"?limit=100&offset=250", 100, 250, true},
		{"?limit=0", 0, 0, false}, {"?limit=101", 0, 0, false},
		{"?offset=-1", 0, 0, false}, {"?offset=1000001", 0, 0, false},
		{"?limit=nope", 0, 0, false},
	}
	for _, tc := range cases {
		r := httptest.NewRequest("GET", "/api/v1/platform/users"+tc.query, nil)
		l, o, ok := parsePlatformPage(r)
		if l != tc.limit || o != tc.offset || ok != tc.ok {
			t.Errorf("parsePlatformPage(%q)=(%d,%d,%v), want (%d,%d,%v)", tc.query, l, o, ok, tc.limit, tc.offset, tc.ok)
		}
	}
}

func TestValidPlatformProvisionInput(t *testing.T) {
	tests := []struct {
		name                                      string
		business, branch, email, currency, reason string
		want                                      bool
	}{
		{"valid", "Harbour Kitchen", "Main Branch", "owner@example.com", "KES", "Approved onboarding case", true},
		{"short business", "A", "Main Branch", "owner@example.com", "KES", "Approved onboarding case", false},
		{"invalid email", "Harbour Kitchen", "Main Branch", "not-an-email", "KES", "Approved onboarding case", false},
		{"invalid currency", "Harbour Kitchen", "Main Branch", "owner@example.com", "K", "Approved onboarding case", false},
		{"non alphabetic currency", "Harbour Kitchen", "Main Branch", "owner@example.com", "K3S", "Approved onboarding case", false},
		{"missing reason", "Harbour Kitchen", "Main Branch", "owner@example.com", "KES", "short", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := validPlatformProvisionInput(tc.business, tc.branch, tc.email, tc.currency, tc.reason); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
