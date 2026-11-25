package main

import (
	"io"
	"net/http"

	"github.com/jackc/pgx/v5"
)

// receiveDarajaSTKCallback durably records a callback against the exact stored
// provider attempt. It deliberately does not mark a payment verified, complete
// a sale, or deduct inventory; those actions require a later provider query.
func (s *server) receiveDarajaSTKCallback(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "invalid_callback_payload"})
		return
	}
	parsed, err := parseDarajaCallback(body)
	if err != nil {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "invalid_callback_payload"})
		return
	}
	fingerprint := darajaCallbackFingerprint(body)
	var businessID, attemptID string
	var duplicate bool
	err = s.db.QueryRow(r.Context(), `SELECT business_id::text,attempt_id::text,duplicate
		FROM ingest_mpesa_stk_callback($1,$2,$3,$4::jsonb)`,
		parsed.CheckoutRequestID, parsed.MerchantRequestID, fingerprint, string(body)).Scan(&businessID, &attemptID, &duplicate)
	if err == pgx.ErrNoRows {
		// Unknown or mismatched provider identifiers are intentionally not
		// disclosed to the caller and are never accepted as a payment result.
		jsonOut(w, http.StatusOK, map[string]any{"ResultCode": 0, "ResultDesc": "Accepted"})
		return
	}
	if err != nil {
		// A transient persistence failure must not be acknowledged: Daraja may
		// retry the callback and the reconciliation path can recover later.
		jsonOut(w, http.StatusServiceUnavailable, map[string]string{"error": "callback_persistence_unavailable"})
		return
	}
	jsonOut(w, http.StatusOK, map[string]any{"ResultCode": 0, "ResultDesc": "Accepted", "duplicate": duplicate})
}
