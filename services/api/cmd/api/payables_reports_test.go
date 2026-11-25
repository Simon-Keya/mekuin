package main

import (
	"net/http/httptest"
	"testing"
)

func TestPayablesAgingBucket(t *testing.T) {
	cases := []struct {
		days int
		want string
	}{{-1, "current"}, {0, "current"}, {1, "1_30"}, {30, "1_30"}, {31, "31_60"}, {60, "31_60"}, {61, "61_90"}, {90, "61_90"}, {91, "over_90"}}
	for _, tc := range cases {
		if got := payablesAgingBucket(tc.days); got != tc.want {
			t.Fatalf("payablesAgingBucket(%d) = %q, want %q", tc.days, got, tc.want)
		}
	}
}

func TestParsePayablesDateRange(t *testing.T) {
	cases := []struct {
		query string
		valid bool
	}{{"?from=2026-01-01&to=2026-01-31", true}, {"?from=2026-02-01&to=2026-01-31", false}, {"?from=2026-02-30", false}, {"?to=2026-02-28", true}, {"", true}}
	for _, tc := range cases {
		r := httptest.NewRequest("GET", "/"+tc.query, nil)
		if tc.query == "" {
			r = httptest.NewRequest("GET", "/", nil)
		}
		_, _, got := parsePayablesDateRange(r)
		if got != tc.valid {
			t.Fatalf("parsePayablesDateRange(%q) valid = %v, want %v", tc.query, got, tc.valid)
		}
	}
}
