package main

import (
	"strings"
	"testing"
)

func TestParseDarajaSuccessfulCallbackExtractsAndConvertsMetadata(t *testing.T) {
	raw := []byte(`{"Body":{"stkCallback":{"MerchantRequestID":"mr_1","CheckoutRequestID":"ws_CO_123","ResultCode":0,"ResultDesc":"Success","CallbackMetadata":{"Item":[{"Name":"Amount","Value":125},{"Name":"MpesaReceiptNumber","Value":"QGH123ABC9"},{"Name":"TransactionDate","Value":20261124001530},{"Name":"PhoneNumber","Value":254712345678}]}}}}`)
	got, err := parseDarajaCallback(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.AmountMinor != 12500 || got.Phone != "254712345678" || got.Receipt != "QGH123ABC9" || got.CheckoutRequestID != "ws_CO_123" {
		t.Fatalf("unexpected parsed callback: %+v", got)
	}
}

func TestParseDarajaCallbackDoesNotTreatFailureAsPayment(t *testing.T) {
	raw := []byte(`{"Body":{"stkCallback":{"MerchantRequestID":"mr_1","CheckoutRequestID":"ws_CO_123","ResultCode":1032,"ResultDesc":"Cancelled"}}}`)
	got, err := parseDarajaCallback(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.ResultCode == 0 || got.Receipt != "" {
		t.Fatalf("unexpected success metadata: %+v", got)
	}
}

func TestParseDarajaCallbackRejectsMissingSuccessMetadataAndDuplicates(t *testing.T) {
	missing := []byte(`{"Body":{"stkCallback":{"MerchantRequestID":"mr_1","CheckoutRequestID":"ws_CO_123","ResultCode":0,"ResultDesc":"Success"}}}`)
	if _, err := parseDarajaCallback(missing); err == nil {
		t.Fatal("expected missing metadata rejection")
	}
	duplicate := []byte(`{"Body":{"stkCallback":{"MerchantRequestID":"mr_1","CheckoutRequestID":"ws_CO_123","ResultCode":1,"CallbackMetadata":{"Item":[{"Name":"Amount","Value":10},{"Name":"Amount","Value":10}]}}}}`)
	if _, err := parseDarajaCallback(duplicate); err == nil {
		t.Fatal("expected duplicate metadata rejection")
	}
}

func TestDarajaCallbackFingerprintIsDeterministic(t *testing.T) {
	raw := []byte(`{"x":1}`)
	if darajaCallbackFingerprint(raw) != darajaCallbackFingerprint(raw) {
		t.Fatal("fingerprint is not deterministic")
	}
	if strings.TrimSpace(darajaCallbackFingerprint(raw)) == "" {
		t.Fatal("empty fingerprint")
	}
}
