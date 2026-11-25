package main

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type mpesaAttemptSummary struct {
	AttemptID           string    `json:"attempt_id"`
	SaleID              string    `json:"sale_id"`
	Status              string    `json:"status"`
	AmountMinor         int64     `json:"amount_minor"`
	Currency            string    `json:"currency"`
	PhoneMasked         string    `json:"phone_masked"`
	Receipt             string    `json:"receipt,omitempty"`
	ProviderDescription string    `json:"provider_description,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	CanReconcile        bool      `json:"can_reconcile"`
	CanSettle           bool      `json:"can_settle"`
}

// listMPesaAttempts exposes only the active tenant's current branch attempts.
// It intentionally omits full MSISDNs, callback payloads and provider secrets.
func (s *server) listMPesaAttempts(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, http.StatusInternalServerError, map[string]string{"error": "tenant_context_missing"})
		return
	}
	limit, offset, _, valid := listBounds(r)
	if !valid {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "invalid_pagination"})
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	allowed := map[string]bool{"created": true, "accepted": true, "callback_received": true, "verification_pending": true, "provider_confirmed": true, "verified": true, "failed": true, "expired": true, "review_required": true}
	if status != "" && !allowed[status] {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "invalid_status_filter"})
		return
	}
	rows, err := tx.Query(r.Context(), `SELECT a.id::text,a.sale_id::text,a.status,a.amount_minor,a.currency,a.phone_e164,
		COALESCE(a.mpesa_receipt_number,''),COALESCE(a.provider_result_description,''),a.created_at,a.updated_at,
		(a.checkout_request_id IS NOT NULL AND a.checkout_request_id<>'' AND a.merchant_request_id IS NOT NULL AND a.merchant_request_id<>''),
		(a.status='provider_confirmed')
		FROM mpesa_payment_attempts a WHERE a.business_id=$1 AND a.branch_id=$2 AND ($3='' OR a.status=$3)
		ORDER BY a.created_at DESC,a.id DESC LIMIT $4 OFFSET $5`, tc.BusinessID, tc.BranchID, status, limit, offset)
	if err != nil {
		catalogError(w, err)
		return
	}
	defer rows.Close()
	items := make([]mpesaAttemptSummary, 0)
	for rows.Next() {
		var item mpesaAttemptSummary
		var phone string
		if err := rows.Scan(&item.AttemptID, &item.SaleID, &item.Status, &item.AmountMinor, &item.Currency, &phone, &item.Receipt, &item.ProviderDescription, &item.CreatedAt, &item.UpdatedAt, &item.CanReconcile, &item.CanSettle); err != nil {
			catalogError(w, err)
			return
		}
		item.PhoneMasked = maskDarajaPhone(phone)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, http.StatusOK, map[string]any{"items": items, "limit": limit, "offset": offset})
}

func maskDarajaPhone(phone string) string {
	p := strings.TrimSpace(phone)
	if len(p) < 7 {
		return "***"
	}
	prefix := 3
	if strings.HasPrefix(p, "+") {
		prefix = 4
	}
	stars := len(p) - 7
	if stars < 1 {
		stars = 1
	}
	return p[:prefix] + strings.Repeat("*", stars) + p[len(p)-3:]
}

// recordMPesaReviewNote records an operator's investigation without changing
// payment, sale or inventory state. It is deliberately not a force-settlement API.
func (s *server) recordMPesaReviewNote(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, http.StatusInternalServerError, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in struct {
		Note string `json:"note"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !requiredText(in.Note, 500) {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "invalid_review_note"})
		return
	}
	attemptID := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "attemptID")))
	if !validUUID(attemptID) {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "invalid_attempt_id"})
		return
	}
	var saleID string
	var status string
	err := tx.QueryRow(r.Context(), `SELECT sale_id::text,status FROM mpesa_payment_attempts WHERE business_id=$1 AND branch_id=$2 AND id=$3 FOR UPDATE`, tc.BusinessID, tc.BranchID, attemptID).Scan(&saleID, &status)
	if err == pgx.ErrNoRows {
		jsonOut(w, http.StatusNotFound, map[string]string{"error": "payment_attempt_not_found"})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	if status != "review_required" {
		jsonOut(w, http.StatusConflict, map[string]string{"error": "attempt_not_in_review"})
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary,metadata) VALUES($1,$2,'payments.mpesa_review_note','mpesa_payment_attempt',$3,'Operator recorded M-Pesa investigation note',jsonb_build_object('sale_id',$4,'note',$5))`, tc.BusinessID, tc.UserID, attemptID, saleID, strings.TrimSpace(in.Note))
	if err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, http.StatusCreated, map[string]string{"attempt_id": attemptID, "status": "review_required", "note_recorded": "true"})
}
