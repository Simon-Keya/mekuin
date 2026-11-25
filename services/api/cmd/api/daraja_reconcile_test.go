package main

import (
	"testing"
	"time"
)

func TestDarajaTransactionTimeUsesEastAfricaOffset(t *testing.T) {
	got := darajaTransactionTime("20261124001530")
	want := time.Date(2026, 11, 23, 21, 15, 30, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestDarajaTransactionTimeRejectsInvalidCalendarDate(t *testing.T) {
	if got := darajaTransactionTime("20261340000000"); !got.IsZero() {
		t.Fatalf("expected invalid date to be rejected, got %s", got)
	}
}

func TestDarajaQueryDisposition(t *testing.T) {
	cases := map[string]string{
		"499": "pending", "4999": "pending", "1001": "pending", "500.001.1001": "pending",
		"1019": "expired", "1025": "failed", "1032": "failed", "1037": "failed",
		"2001": "failed", "9999": "failed", "1": "failed",
		"0": "review_required", "unknown-provider-code": "review_required",
	}
	for code, want := range cases {
		if got := darajaQueryDisposition(code); got != want {
			t.Errorf("darajaQueryDisposition(%q) = %q, want %q", code, got, want)
		}
	}
}
