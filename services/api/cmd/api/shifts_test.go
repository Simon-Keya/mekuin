package main

import "testing"

func TestCashShiftExpectedCash(t *testing.T) {
	got := cashShiftExpectedCash(200000, 150000, 30000, 10000)
	if got != 370000 {
		t.Fatalf("expected drawer cash = %d, want 370000", got)
	}
	variance := 365000 - got
	if variance != -5000 {
		t.Fatalf("variance = %d, want -5000", variance)
	}
}
