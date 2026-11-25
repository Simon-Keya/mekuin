package main

import "testing"

func TestDarajaSettlementEligibility(t *testing.T) {
	cases := []struct {
		name                   string
		attempt, sale, payment string
		amount, paymentAmount  int64
		receipt                string
		want                   bool
	}{
		{"matching provider-confirmed pending sale", "provider_confirmed", "pending_payment", "pending_verification", 12500, 12500, "QWE123", true},
		{"callback alone is insufficient", "callback_received", "pending_payment", "pending_verification", 12500, 12500, "QWE123", false},
		{"query pending is insufficient", "verification_pending", "pending_payment", "pending_verification", 12500, 12500, "QWE123", false},
		{"completed sale cannot be charged again", "provider_confirmed", "completed", "pending_verification", 12500, 12500, "QWE123", false},
		{"payment already verified", "provider_confirmed", "pending_payment", "verified", 12500, 12500, "QWE123", false},
		{"amount mismatch", "provider_confirmed", "pending_payment", "pending_verification", 12500, 12000, "QWE123", false},
		{"missing receipt", "provider_confirmed", "pending_payment", "pending_verification", 12500, 12500, "", false},
		{"nonpositive amount", "provider_confirmed", "pending_payment", "pending_verification", 0, 0, "QWE123", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := darajaSettlementEligible(tc.attempt, tc.sale, tc.payment, tc.amount, tc.paymentAmount, tc.receipt)
			if got != tc.want {
				t.Fatalf("eligible=%v want %v", got, tc.want)
			}
		})
	}
}
