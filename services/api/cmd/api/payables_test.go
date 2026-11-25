package main

import "testing"

func TestValidCurrencyCode(t *testing.T) {
	for _, value := range []string{"KES", "USD", "EUR"} {
		if !validCurrencyCode(value) {
			t.Fatalf("expected %q to be valid", value)
		}
	}
	for _, value := range []string{"kes", "KE", "KESH", "K1S", " KES"} {
		if validCurrencyCode(value) {
			t.Fatalf("expected %q to be invalid", value)
		}
	}
}

func TestValidISODate(t *testing.T) {
	for _, value := range []string{"2026-01-01", "2024-02-29"} {
		if !validISODate(value, false) {
			t.Fatalf("expected %q to be valid", value)
		}
	}
	for _, value := range []string{"2025-02-29", "01-02-2026", "2026-1-01", ""} {
		if validISODate(value, false) {
			t.Fatalf("expected %q to be invalid", value)
		}
	}
	if !validISODate("", true) {
		t.Fatal("empty optional date should be valid")
	}
}

func TestSumMinorDetectsOverflow(t *testing.T) {
	got, ok := sumMinor([]int64{100, 250, 650})
	if !ok || got != 1000 {
		t.Fatalf("got (%d,%v), want (1000,true)", got, ok)
	}
	if _, ok := sumMinor([]int64{int64(^uint64(0) >> 1), 1}); ok {
		t.Fatal("expected overflow to be rejected")
	}
}

func TestEffectiveInvoiceStatus(t *testing.T) {
	cases := []struct {
		stored      string
		paid, total int64
		want        string
	}{
		{"recorded", 0, 1000, "recorded"},
		{"recorded", 300, 1000, "partially_paid"},
		{"recorded", 1000, 1000, "paid"},
		{"recorded", 1200, 1000, "paid"},
		{"void", 0, 1000, "void"},
	}
	for _, tc := range cases {
		if got := effectiveInvoiceStatus(tc.stored, tc.paid, tc.total); got != tc.want {
			t.Errorf("effectiveInvoiceStatus(%q,%d,%d)=%q, want %q", tc.stored, tc.paid, tc.total, got, tc.want)
		}
	}
}

func TestValidateInvoiceReviewDecision(t *testing.T) {
	cases := []struct {
		name, decision, reason string
		exceptions             bool
		want                   string
	}{
		{"clean approval", "approve", "", false, ""},
		{"approval cannot ignore exceptions", "approve", "", true, "invoice_has_matching_exceptions"},
		{"exception approval needs reason", "approve_with_exceptions", "", true, "exception_resolution_reason_required"},
		{"exception approval with reason", "approve_with_exceptions", "Accepted freight variance", true, ""},
		{"exception approval without exception", "approve_with_exceptions", "Reviewed", false, "invoice_has_no_matching_exceptions"},
		{"rejection needs reason", "reject", "", false, "rejection_reason_required"},
		{"rejection with reason", "reject", "Incorrect supplier amount", false, ""},
		{"unknown decision", "skip", "", false, "invalid_review_decision"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validateInvoiceReviewDecision(tc.decision, tc.reason, tc.exceptions); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestInvoiceReviewAllowsPayment(t *testing.T) {
	for _, status := range []string{"approved", "approved_with_exceptions"} {
		if !invoiceReviewAllowsPayment(status) {
			t.Errorf("%q should allow payment", status)
		}
	}
	for _, status := range []string{"not_submitted", "submitted", "rejected", "recorded", ""} {
		if invoiceReviewAllowsPayment(status) {
			t.Errorf("%q must not allow payment", status)
		}
	}
}
