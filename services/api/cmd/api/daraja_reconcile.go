package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// reconcileDarajaAttempt queries Safaricom for an already-correlated STK
// attempt. Provider confirmation is deliberately distinct from settlement:
// this endpoint never marks a sale completed or writes inventory movements.
func (s *server) reconcileDarajaAttempt(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, http.StatusInternalServerError, map[string]string{"error": "tenant_context_missing"})
		return
	}
	attemptID := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "attemptID")))
	if !validUUID(attemptID) {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "invalid_attempt_id"})
		return
	}
	var checkoutText, merchantText pgtype.Text
	var environment, status string
	var amount int64
	var phone string
	err := tx.QueryRow(r.Context(), `SELECT checkout_request_id,merchant_request_id,environment,status,amount_minor,phone_e164
		FROM mpesa_payment_attempts WHERE business_id=$1 AND branch_id=$2 AND id=$3 FOR UPDATE`, tc.BusinessID, tc.BranchID, attemptID).
		Scan(&checkoutText, &merchantText, &environment, &status, &amount, &phone)
	if errors.Is(err, pgx.ErrNoRows) {
		jsonOut(w, http.StatusNotFound, map[string]string{"error": "payment_attempt_not_found"})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	if !checkoutText.Valid || !merchantText.Valid || checkoutText.String == "" || merchantText.String == "" {
		jsonOut(w, http.StatusConflict, map[string]string{"error": "provider_request_not_accepted_yet"})
		return
	}
	if status == "verified" || status == "provider_confirmed" || status == "failed" || status == "expired" {
		jsonOut(w, http.StatusConflict, map[string]string{"error": "payment_attempt_not_reconcilable", "status": status})
		return
	}

	checkoutID, merchantID := checkoutText.String, merchantText.String
	client, err := newDarajaClientFromEnv()
	if err != nil {
		jsonOut(w, http.StatusServiceUnavailable, map[string]string{"error": "daraja_not_configured"})
		return
	}
	if (environment == "sandbox") != strings.Contains(client.baseURL, "sandbox.safaricom.co.ke") {
		jsonOut(w, http.StatusServiceUnavailable, map[string]string{"error": "daraja_environment_mismatch"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	result, err := client.querySTK(ctx, checkoutID, time.Now())
	if err != nil {
		jsonOut(w, http.StatusBadGateway, map[string]string{"error": "daraja_status_query_failed"})
		return
	}
	if result.MerchantRequestID != "" && result.MerchantRequestID != merchantID {
		jsonOut(w, http.StatusConflict, map[string]string{"error": "provider_request_identity_mismatch"})
		return
	}

	// Query result code 0 is provider-side confirmation of a successful STK
	// transaction. It is not yet settlement: the callback metadata must also
	// match the stored amount and phone, and a later atomic settlement step is
	// responsible for the sale and recipe-stock transaction.
	if result.ResultCode == "" {
		_, err = tx.Exec(r.Context(), `UPDATE mpesa_payment_attempts SET status='verification_pending',provider_result_description=$4,provider_queried_at=now(),provider_query_result_code=NULL,provider_query_description=$4,updated_at=now()
			WHERE business_id=$1 AND branch_id=$2 AND id=$3`, tc.BusinessID, tc.BranchID, attemptID, boundedText(result.ResultDesc, 500))
		if err != nil {
			catalogError(w, err)
			return
		}
		jsonOut(w, http.StatusOK, map[string]any{"attempt_id": attemptID, "status": "verification_pending", "provider_result": "pending"})
		return
	}
	if result.ResultCode != "0" {
		disposition := darajaQueryDisposition(result.ResultCode)
		if disposition == "pending" {
			_, err = tx.Exec(r.Context(), `UPDATE mpesa_payment_attempts SET status='verification_pending',provider_result_code=$4,provider_result_description=$5,provider_queried_at=now(),provider_query_result_code=$4,provider_query_description=$5,updated_at=now()
				WHERE business_id=$1 AND branch_id=$2 AND id=$3`, tc.BusinessID, tc.BranchID, attemptID, result.ResultCode, boundedText(result.ResultDesc, 500))
			if err != nil {
				catalogError(w, err)
				return
			}
			jsonOut(w, http.StatusOK, map[string]any{"attempt_id": attemptID, "status": "verification_pending", "provider_result_code": result.ResultCode})
			return
		}
		if disposition == "review_required" {
			_, err = tx.Exec(r.Context(), `UPDATE mpesa_payment_attempts SET status='review_required',provider_result_code=$4,provider_result_description=$5,provider_queried_at=now(),provider_query_result_code=$4,provider_query_description=$5,updated_at=now()
				WHERE business_id=$1 AND branch_id=$2 AND id=$3`, tc.BusinessID, tc.BranchID, attemptID, result.ResultCode, boundedText(result.ResultDesc, 500))
			if err != nil {
				catalogError(w, err)
				return
			}
			jsonOut(w, http.StatusOK, map[string]any{"attempt_id": attemptID, "status": "review_required", "provider_result_code": result.ResultCode})
			return
		}
		terminalStatus := disposition // failed or expired, based on known STK result codes only
		_, err = tx.Exec(r.Context(), `UPDATE mpesa_payment_attempts SET status=$4,provider_result_code=$5,provider_result_description=$6,failed_at=now(),provider_queried_at=now(),provider_query_result_code=$5,provider_query_description=$6,updated_at=now()
			WHERE business_id=$1 AND branch_id=$2 AND id=$3`, tc.BusinessID, tc.BranchID, attemptID, terminalStatus, result.ResultCode, boundedText(result.ResultDesc, 500))
		if err != nil {
			catalogError(w, err)
			return
		}
		jsonOut(w, http.StatusOK, map[string]any{"attempt_id": attemptID, "status": terminalStatus, "provider_result_code": result.ResultCode, "provider_result_description": boundedText(result.ResultDesc, 500)})
		return
	}

	var callbackPayload []byte
	err = tx.QueryRow(r.Context(), `SELECT payload FROM mpesa_callback_events WHERE business_id=$1 AND attempt_id=$2 ORDER BY received_at DESC LIMIT 1`, tc.BusinessID, attemptID).Scan(&callbackPayload)
	if errors.Is(err, pgx.ErrNoRows) {
		_, err = tx.Exec(r.Context(), `UPDATE mpesa_payment_attempts SET status='verification_pending',provider_result_code='0',provider_result_description='Provider reports success; awaiting callback metadata',provider_queried_at=now(),provider_query_result_code='0',provider_query_description='Provider reports success; awaiting callback metadata',updated_at=now()
			WHERE business_id=$1 AND branch_id=$2 AND id=$3`, tc.BusinessID, tc.BranchID, attemptID)
		if err != nil {
			catalogError(w, err)
			return
		}
		jsonOut(w, http.StatusOK, map[string]any{"attempt_id": attemptID, "status": "verification_pending", "reason": "callback_metadata_missing"})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	callback, parseErr := parseDarajaCallback(callbackPayload)
	if parseErr != nil || callback.ResultCode != 0 || callback.AmountMinor != amount || callback.Phone != phone || callback.Receipt == "" || callback.CheckoutRequestID != checkoutID || callback.MerchantRequestID != merchantID || darajaTransactionTime(callback.TransactionDate).IsZero() {
		_, updateErr := tx.Exec(r.Context(), `UPDATE mpesa_payment_attempts SET status='review_required',provider_result_code='0',provider_result_description='Provider success did not match stored callback metadata',provider_queried_at=now(),provider_query_result_code='0',provider_query_description='Provider success; callback metadata mismatch',updated_at=now()
			WHERE business_id=$1 AND branch_id=$2 AND id=$3`, tc.BusinessID, tc.BranchID, attemptID)
		if updateErr != nil {
			catalogError(w, updateErr)
			return
		}
		jsonOut(w, http.StatusConflict, map[string]string{"error": "provider_callback_metadata_mismatch", "status": "review_required"})
		return
	}
	_, err = tx.Exec(r.Context(), `UPDATE mpesa_payment_attempts SET status='provider_confirmed',provider_result_code='0',provider_result_description=$4,mpesa_receipt_number=$5,transaction_at=$6,provider_queried_at=now(),provider_query_result_code='0',provider_query_description=$4,callback_metadata_matched_at=now(),updated_at=now()
		WHERE business_id=$1 AND branch_id=$2 AND id=$3`, tc.BusinessID, tc.BranchID, attemptID, boundedText(result.ResultDesc, 500), callback.Receipt, darajaTransactionTime(callback.TransactionDate))
	if err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, http.StatusOK, map[string]any{"attempt_id": attemptID, "status": "provider_confirmed", "receipt": callback.Receipt, "settlement": "not_yet_performed"})
}

// Daraja callback transaction dates use yyyyMMddHHmmss in East Africa time.
func darajaTransactionTime(value string) time.Time {
	t, err := time.ParseInLocation("20060102150405", value, time.FixedZone("EAT", 3*60*60))
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

// darajaQueryDisposition is deliberately conservative. Only documented/common
// STK terminal codes are finalized; known in-progress responses remain pending
// and unrecognized values require operator review rather than false failure.
func darajaQueryDisposition(code string) string {
	switch strings.TrimSpace(code) {
	case "499", "4999", "1001", "500.001.1001":
		// These responses indicate that the subscriber/session or provider
		// transaction is still in progress. Do not turn them into a failed sale.
		return "pending"
	case "1019":
		return "expired"
	case "1", "1025", "1032", "1037", "2001", "9999":
		// Known STK terminal outcomes. Unknown codes remain review_required.
		return "failed"
	default:
		return "review_required"
	}
}
