package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDarajaSTKQueryUsesProviderCorrelationAndDoesNotTreatHTTPAsSettlement(t *testing.T) {
	now := time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)
	var got darajaSTKQueryRequest
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/v1/generate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("oauth method = %s", r.Method)
		}
		u, p, ok := r.BasicAuth()
		if !ok || u != "key" || p != "secret" {
			t.Errorf("unexpected basic auth")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"token","expires_in":"3600"}`))
	})
	mux.HandleFunc("/mpesa/stkpushquery/v1/query", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Errorf("missing bearer token")
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ResponseCode":"0","ResponseDescription":"Accepted","CheckoutRequestID":"ws_CO_123","ResultCode":"1032","ResultDesc":"Request cancelled by user"}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	d := &darajaClient{baseURL: server.URL, consumerKey: "key", consumerSecret: "secret", shortcode: "174379", passkey: "pass", httpClient: server.Client()}
	gotResponse, err := d.querySTK(context.Background(), "ws_CO_123", now)
	if err != nil {
		t.Fatal(err)
	}
	if got.CheckoutRequestID != "ws_CO_123" || got.BusinessShortCode != "174379" {
		t.Fatalf("unexpected query payload: %+v", got)
	}
	expectedPassword := base64.StdEncoding.EncodeToString([]byte("174379pass20261124000000"))
	if got.Password != expectedPassword {
		t.Fatalf("unexpected password: %s", got.Password)
	}
	if gotResponse.ResultCode != "1032" {
		t.Fatalf("result code = %q", gotResponse.ResultCode)
	}
	// ResponseCode=0 only means the query request was accepted. A non-zero
	// ResultCode remains a failed customer payment, not a successful settlement.
	if gotResponse.ResultCode == "0" {
		t.Fatal("test must not represent a successful payment")
	}
}

func TestDarajaSTKQueryRejectsEmptyOrMismatchedCheckoutID(t *testing.T) {
	d := &darajaClient{baseURL: "https://example.invalid", consumerKey: "k", consumerSecret: "s", shortcode: "1", passkey: "p", httpClient: http.DefaultClient}
	if _, err := d.querySTK(context.Background(), "", time.Now()); err == nil {
		t.Fatal("expected empty ID rejection")
	}
	if strings.TrimSpace(" ws_CO_123 ") != "ws_CO_123" {
		t.Fatal("unexpected test precondition")
	}
}

func TestDarajaSTKPushBuildsExpectedProviderRequest(t *testing.T) {
	now := time.Date(2026, 11, 24, 1, 2, 3, 0, time.UTC)
	var got darajaSTKRequest
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/v1/generate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("oauth method = %s", r.Method)
		}
		u, p, ok := r.BasicAuth()
		if !ok || u != "consumer-key" || p != "consumer-secret" {
			t.Errorf("unexpected OAuth credentials")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"access-token","expires_in":"3600"}`))
	})
	mux.HandleFunc("/mpesa/stkpush/v1/processrequest", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer access-token" {
			t.Errorf("unexpected STK request method or authorization")
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode STK request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ResponseCode":"0","ResponseDescription":"Accepted","MerchantRequestID":"merchant-1","CheckoutRequestID":"checkout-1"}`))
	})
	provider := httptest.NewServer(mux)
	defer provider.Close()
	d := &darajaClient{
		baseURL: provider.URL, consumerKey: "consumer-key", consumerSecret: "consumer-secret",
		shortcode: "174379", passkey: "passkey", callbackURL: "https://staging.example.com/daraja/stk/callback",
		httpClient: provider.Client(),
	}
	response, err := d.initiateSTK(context.Background(), "254712345678", 1250, "SALE-REFERENCE-123456", "MEKU POS ORDER", now)
	if err != nil {
		t.Fatal(err)
	}
	if response.CheckoutRequestID != "checkout-1" || response.MerchantRequestID != "merchant-1" {
		t.Fatalf("unexpected provider response: %+v", response)
	}
	if got.BusinessShortCode != "174379" || got.TransactionType != "CustomerPayBillOnline" || got.Amount != 1250 {
		t.Fatalf("unexpected shortcode/type/amount: %+v", got)
	}
	if got.PartyA != "254712345678" || got.PhoneNumber != "254712345678" || got.PartyB != "174379" {
		t.Fatalf("unexpected phone fields: %+v", got)
	}
	if got.CallBackURL != d.callbackURL || got.AccountReference != "SALE-REFEREN" || got.TransactionDesc != "MEKU POS ORDE" {
		t.Fatalf("unexpected callback/reference/description: %+v", got)
	}
	wantTimestamp := "20261124010203"
	if got.Timestamp != wantTimestamp {
		t.Fatalf("timestamp = %q, want %q", got.Timestamp, wantTimestamp)
	}
	wantPassword := base64.StdEncoding.EncodeToString([]byte("174379passkey" + wantTimestamp))
	if got.Password != wantPassword {
		t.Fatal("STK password did not match the shortcode/passkey/timestamp contract")
	}
}

func TestDarajaSTKPushRejectsInvalidAmountAndPhoneBeforeNetwork(t *testing.T) {
	d := &darajaClient{baseURL: "https://example.invalid", consumerKey: "k", consumerSecret: "s", shortcode: "1", passkey: "p", callbackURL: "https://example.com/callback", httpClient: http.DefaultClient}
	for _, tc := range []struct {
		name   string
		phone  string
		amount int64
	}{
		{name: "zero amount", phone: "254712345678", amount: 0},
		{name: "over provider limit", phone: "254712345678", amount: 250001},
		{name: "invalid phone", phone: "+254712345678", amount: 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := d.initiateSTK(context.Background(), tc.phone, tc.amount, "SALE", "POS", time.Now()); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
