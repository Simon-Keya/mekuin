package main

import "testing"

func TestCanonicalReceiptFingerprintNormalizesEquivalentPayloads(t *testing.T) {
	lineA := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	lineB := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	first := receivePurchaseInput{Note: "  delivery  ", Lines: []struct {
		LineID   string `json:"purchase_order_line_id"`
		Quantity string `json:"quantity"`
	}{{LineID: lineA, Quantity: "01.5000"}, {LineID: lineB, Quantity: "2.000"}}}
	second := receivePurchaseInput{Note: "delivery", Lines: []struct {
		LineID   string `json:"purchase_order_line_id"`
		Quantity string `json:"quantity"`
	}{{LineID: " " + lineB + " ", Quantity: "2"}, {LineID: "AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA", Quantity: "1.5"}}}

	_, firstFingerprint, firstOK := canonicalReceiptFingerprint(first)
	_, secondFingerprint, secondOK := canonicalReceiptFingerprint(second)
	if !firstOK || !secondOK {
		t.Fatal("equivalent valid receipts should canonicalize")
	}
	if firstFingerprint != secondFingerprint {
		t.Fatalf("equivalent payload fingerprints differ: %s != %s", firstFingerprint, secondFingerprint)
	}

	changed := first
	changed.Lines = append([]struct {
		LineID   string `json:"purchase_order_line_id"`
		Quantity string `json:"quantity"`
	}{{LineID: lineA, Quantity: "1.6"}, {LineID: lineB, Quantity: "2"}})
	_, changedFingerprint, changedOK := canonicalReceiptFingerprint(changed)
	if !changedOK || changedFingerprint == firstFingerprint {
		t.Fatal("a changed receipt quantity must produce a different fingerprint")
	}
}

func TestCanonicalReceiptFingerprintRejectsInvalidLines(t *testing.T) {
	validID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	tests := []struct {
		name  string
		lines []struct {
			LineID   string `json:"purchase_order_line_id"`
			Quantity string `json:"quantity"`
		}
	}{
		{name: "zero quantity", lines: []struct {
			LineID   string `json:"purchase_order_line_id"`
			Quantity string `json:"quantity"`
		}{{LineID: validID, Quantity: "0.000"}}},
		{name: "duplicate line", lines: []struct {
			LineID   string `json:"purchase_order_line_id"`
			Quantity string `json:"quantity"`
		}{{LineID: validID, Quantity: "1"}, {LineID: validID, Quantity: "2"}}},
		{name: "too much precision", lines: []struct {
			LineID   string `json:"purchase_order_line_id"`
			Quantity string `json:"quantity"`
		}{{LineID: validID, Quantity: "1.123456789"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := receivePurchaseInput{Lines: tt.lines}
			if _, _, ok := canonicalReceiptFingerprint(in); ok {
				t.Fatal("invalid receipt unexpectedly accepted")
			}
		})
	}
}

func TestPurchaseIdempotencyKeyContract(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"12345678", true},
		{" 12345678 ", true},
		{"short", false},
		{"contains space", false},
		{"newline\nkey123", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := validPurchaseIdempotencyKey(tt.key); got != tt.want {
			t.Errorf("validPurchaseIdempotencyKey(%q)=%v want %v", tt.key, got, tt.want)
		}
	}
}

func TestPurchaseOrderLifecyclePredicates(t *testing.T) {
	for _, tt := range []struct {
		status      string
		receivable  bool
		cancellable bool
	}{
		{"draft", false, true},
		{"ordered", true, true},
		{"partially_received", true, false},
		{"received", false, false},
		{"cancelled", false, false},
		{"unknown", false, false},
	} {
		if got := purchaseOrderReceivable(tt.status); got != tt.receivable {
			t.Errorf("receivable(%q)=%v want %v", tt.status, got, tt.receivable)
		}
		if got := purchaseOrderCancellable(tt.status); got != tt.cancellable {
			t.Errorf("cancellable(%q)=%v want %v", tt.status, got, tt.cancellable)
		}
	}
}
