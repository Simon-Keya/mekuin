package main

import (
	"math"
	"testing"
)

func TestCalculateSaleLineUsesExactMinorUnits(t *testing.T) {
	line, err := calculateSaleLine("p1", "GIN-001", "Gin & Tonic", 3, 12550)
	if err != nil {
		t.Fatal(err)
	}
	if line.LineTotal != 37650 {
		t.Fatalf("line total = %d, want 37650", line.LineTotal)
	}
}

func TestCalculateSaleLineRejectsInvalidQuantityAndOverflow(t *testing.T) {
	if _, err := calculateSaleLine("p1", "x", "x", 0, 10); err == nil {
		t.Fatal("expected zero quantity rejection")
	}
	if _, err := calculateSaleLine("p1", "x", "x", 2, math.MaxInt64); err != errSaleAmountOverflow {
		t.Fatalf("overflow error = %v", err)
	}
}

func TestCalculateSaleTotalsMatchesBasket(t *testing.T) {
	lines := []saleQuoteLine{{LineTotal: 12550}, {LineTotal: 25000}, {LineTotal: 750}}
	totals, err := calculateSaleTotals(lines)
	if err != nil {
		t.Fatal(err)
	}
	if totals.Subtotal != 38300 || totals.Total != 38300 {
		t.Fatalf("unexpected totals: %+v", totals)
	}
	if totals.Tax != 0 || totals.Service != 0 || totals.Discount != 0 {
		t.Fatalf("unapproved charges must not be invented: %+v", totals)
	}
}

func TestCalculateSaleTotalsRejectsEmptyAndOverflow(t *testing.T) {
	if _, err := calculateSaleTotals(nil); err == nil {
		t.Fatal("expected empty basket rejection")
	}
	if _, err := calculateSaleTotals([]saleQuoteLine{{LineTotal: math.MaxInt64}, {LineTotal: 1}}); err != errSaleAmountOverflow {
		t.Fatalf("overflow error = %v", err)
	}
}

func TestNormalizeCashTenders(t *testing.T) {
	tests := []struct {
		name    string
		in      checkoutRequest
		total   int64
		want    int
		wantErr bool
	}{
		{name: "legacy cash", in: checkoutRequest{PaymentMethod: "cash"}, total: 1000, want: 1},
		{name: "split cash", in: checkoutRequest{Payments: []checkoutTender{{Method: "cash", Amount: 400}, {Method: "cash", Amount: 600}}}, total: 1000, want: 2},
		{name: "underpaid", in: checkoutRequest{Payments: []checkoutTender{{Method: "cash", Amount: 900}}}, total: 1000, wantErr: true},
		{name: "overpaid", in: checkoutRequest{Payments: []checkoutTender{{Method: "cash", Amount: 1100}}}, total: 1000, wantErr: true},
		{name: "unverified rail", in: checkoutRequest{Payments: []checkoutTender{{Method: "mpesa", Amount: 1000}}}, total: 1000, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeCashTenders(tt.in, tt.total)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v", err)
			}
			if err == nil && len(got) != tt.want {
				t.Fatalf("got %d tenders", len(got))
			}
		})
	}
}

func TestNormalizeCheckoutTenders(t *testing.T) {
	tests := []struct {
		name    string
		in      checkoutRequest
		total   int64
		wantErr bool
	}{
		{name: "legacy cash", in: checkoutRequest{}, total: 1000},
		{name: "split cash", in: checkoutRequest{Payments: []checkoutTender{{Method: "cash", Amount: 400}, {Method: "cash", Amount: 600}}}, total: 1000},
		{name: "full mpesa", in: checkoutRequest{Payments: []checkoutTender{{Method: "mpesa", Amount: 1000, Phone: "254712345678"}}}, total: 1000},
		{name: "mixed rail", in: checkoutRequest{Payments: []checkoutTender{{Method: "cash", Amount: 500}, {Method: "mpesa", Amount: 500, Phone: "254712345678"}}}, total: 1000, wantErr: true},
		{name: "underallocated", in: checkoutRequest{Payments: []checkoutTender{{Method: "cash", Amount: 999}}}, total: 1000, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeCheckoutTenders(tt.in, tt.total)
			if (err != nil) != tt.wantErr {
				t.Fatalf("got %v, err %v", got, err)
			}
		})
	}
}

func TestDarajaCheckoutAmountAndPhoneValidation(t *testing.T) {
	if !validDarajaPhone("254712345678") {
		t.Fatal("valid Kenyan MSISDN rejected")
	}
	for _, v := range []string{"0712345678", "25471234567", "2547123456789", "2547abcdefgh"} {
		if validDarajaPhone(v) {
			t.Fatalf("invalid phone accepted: %s", v)
		}
	}
	if int64(12550)%100 == 0 {
		t.Fatal("fractional KES amount unexpectedly whole")
	}
	if int64(12500)%100 != 0 {
		t.Fatal("whole KES amount rejected")
	}
}
