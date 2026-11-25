package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// darajaCallbackEnvelope is the documented STK Push callback envelope. The
// callback is treated as an untrusted notification until it is correlated to
// a stored attempt and independently checked with STK Push Query.
type darajaCallbackEnvelope struct {
	Body struct {
		STKCallback struct {
			MerchantRequestID string `json:"MerchantRequestID"`
			CheckoutRequestID string `json:"CheckoutRequestID"`
			ResultCode        int64  `json:"ResultCode"`
			ResultDesc        string `json:"ResultDesc"`
			Metadata          struct {
				Items []struct {
					Name  string `json:"Name"`
					Value any    `json:"Value"`
				} `json:"Item"`
			} `json:"CallbackMetadata"`
		} `json:"stkCallback"`
	} `json:"Body"`
}

type darajaCallbackResult struct {
	MerchantRequestID string
	CheckoutRequestID string
	ResultCode        int64
	ResultDescription string
	AmountMinor       int64
	Phone             string
	Receipt           string
	TransactionDate   string
}

// parseDarajaCallback extracts provider fields without asserting settlement.
// Amount is expected as a whole KES value by Daraja STK Push; the stored sale
// amount is also in minor units, so callers must compare AmountMinor*100 only
// after overflow-safe conversion and currency policy validation.
func parseDarajaCallback(raw []byte) (darajaCallbackResult, error) {
	var out darajaCallbackResult
	if len(raw) == 0 || len(raw) > 64<<10 {
		return out, errors.New("invalid callback payload size")
	}
	var envelope darajaCallbackEnvelope
	decErr := json.Unmarshal(raw, &envelope)
	if decErr != nil {
		return out, fmt.Errorf("invalid Daraja callback JSON: %w", decErr)
	}
	cb := envelope.Body.STKCallback
	out.MerchantRequestID = strings.TrimSpace(cb.MerchantRequestID)
	out.CheckoutRequestID = strings.TrimSpace(cb.CheckoutRequestID)
	out.ResultCode = cb.ResultCode
	out.ResultDescription = boundedText(cb.ResultDesc, 500)
	if out.MerchantRequestID == "" || out.CheckoutRequestID == "" || len(out.CheckoutRequestID) > 128 {
		return darajaCallbackResult{}, errors.New("callback provider request identifiers are required")
	}
	seen := map[string]bool{}
	for _, item := range cb.Metadata.Items {
		name := strings.TrimSpace(item.Name)
		if name == "" || seen[name] {
			return darajaCallbackResult{}, errors.New("callback metadata contains an empty or duplicate field")
		}
		seen[name] = true
		switch name {
		case "Amount":
			n, ok := numericValue(item.Value)
			if !ok || n <= 0 || n > 250000 {
				return darajaCallbackResult{}, errors.New("callback amount is invalid")
			}
			out.AmountMinor = n * 100
		case "MpesaReceiptNumber":
			v, ok := item.Value.(string)
			if !ok || len(strings.TrimSpace(v)) < 8 || len(strings.TrimSpace(v)) > 20 {
				return darajaCallbackResult{}, errors.New("callback receipt is invalid")
			}
			out.Receipt = strings.TrimSpace(v)
		case "PhoneNumber":
			n, ok := numericValue(item.Value)
			if !ok {
				return darajaCallbackResult{}, errors.New("callback phone is invalid")
			}
			out.Phone = fmt.Sprintf("%d", n)
			if !validDarajaPhone(out.Phone) {
				return darajaCallbackResult{}, errors.New("callback phone is invalid")
			}
		case "TransactionDate":
			n, ok := numericValue(item.Value)
			if !ok || n < 20000101000000 || n > 29991231235959 {
				return darajaCallbackResult{}, errors.New("callback transaction date is invalid")
			}
			out.TransactionDate = fmt.Sprintf("%014d", n)
		}
	}
	if out.ResultCode == 0 && (out.AmountMinor == 0 || out.Phone == "" || out.Receipt == "" || out.TransactionDate == "") {
		return darajaCallbackResult{}, errors.New("successful callback omitted required payment metadata")
	}
	return out, nil
}

func numericValue(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		if n < 0 || n > 1e15 || n != float64(int64(n)) {
			return 0, false
		}
		return int64(n), true
	case json.Number:
		var out int64
		_, err := fmt.Sscan(string(n), &out)
		return out, err == nil && out >= 0
	default:
		return 0, false
	}
}

// darajaCallbackFingerprint is stable for an exact raw callback body. It is
// useful for idempotent event storage; it is not an authenticity signature.
func darajaCallbackFingerprint(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
