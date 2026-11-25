package main

import "testing"

func TestMaskDarajaPhone(t *testing.T) {
	cases := map[string]string{
		"+254712345678": "+254******678",
		"254712345678":  "254*****678",
		"123":           "***",
		"":              "***",
	}
	for input, want := range cases {
		if got := maskDarajaPhone(input); got != want {
			t.Fatalf("maskDarajaPhone(%q)=%q want %q", input, got, want)
		}
	}
}
