package main

import "testing"

func TestExpenseReviewValidation(t *testing.T) {
	tests := []struct{ decision, reason, want string }{
		{"approve", "", ""}, {"approve", "manager note", ""}, {"reject", "too short", "invalid_review_reason"}, {"reject", "duplicate receipt", ""}, {"other", "reason", "invalid_review_decision"}, {"reject", string(make([]byte, 2001)), "invalid_review_reason"},
	}
	for _, tt := range tests {
		if got := validateExpenseReview(tt.decision, tt.reason); got != tt.want {
			t.Errorf("validateExpenseReview(%q,%d chars)=%q want %q", tt.decision, len(tt.reason), got, tt.want)
		}
	}
}
func TestExpensePaymentMethods(t *testing.T) {
	for _, m := range []string{"cash", "bank_transfer", "mobile_money", "card", "other"} {
		if !validExpensePaymentMethod(m) {
			t.Errorf("%s should be valid", m)
		}
	}
	for _, m := range []string{"", "cheque", "crypto", "CASH"} {
		if validExpensePaymentMethod(m) {
			t.Errorf("%s should be invalid", m)
		}
	}
}
