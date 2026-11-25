package main

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// settleDarajaAttempt is the only transition from provider-confirmed M-Pesa
// evidence to a completed sale. Payment, sale, stock movements, order linkage,
// attempt state and audit are committed by the surrounding tenant transaction.
func (s *server) settleDarajaAttempt(w http.ResponseWriter, r *http.Request) {
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

	var saleID, paymentID, attemptStatus, saleStatus, paymentStatus, paymentMethod, attemptCurrency, businessCurrency string
	var amount, saleTotal, paymentAmount int64
	var receipt string
	err := tx.QueryRow(r.Context(), `
		SELECT a.sale_id::text,a.sale_payment_id::text,a.status,s.status,p.status,p.method,
		       a.amount_minor,s.total_minor,p.amount_minor,COALESCE(a.mpesa_receipt_number,''),a.currency,b.currency
		FROM mpesa_payment_attempts a
		JOIN sales s ON s.business_id=a.business_id AND s.id=a.sale_id
		JOIN businesses b ON b.id=a.business_id
		JOIN sale_payments p ON p.business_id=a.business_id AND p.branch_id=a.branch_id AND p.id=a.sale_payment_id AND p.sale_id=a.sale_id AND p.method='mpesa'
		WHERE a.business_id=$1 AND a.branch_id=$2 AND a.id=$3
		FOR UPDATE OF a,s,p`, tc.BusinessID, tc.BranchID, attemptID).
		Scan(&saleID, &paymentID, &attemptStatus, &saleStatus, &paymentStatus, &paymentMethod, &amount, &saleTotal, &paymentAmount, &receipt, &attemptCurrency, &businessCurrency)
	if errors.Is(err, pgx.ErrNoRows) {
		jsonOut(w, http.StatusNotFound, map[string]string{"error": "payment_attempt_not_found"})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	if attemptStatus == "verified" && saleStatus == "completed" && paymentStatus == "verified" {
		jsonOut(w, http.StatusOK, map[string]any{"attempt_id": attemptID, "sale_id": saleID, "status": "settled", "idempotent_replay": true})
		return
	}
	if paymentMethod != "mpesa" || attemptCurrency != businessCurrency || !darajaSettlementEligible(attemptStatus, saleStatus, paymentStatus, amount, paymentAmount, receipt) {
		jsonOut(w, http.StatusConflict, map[string]string{"error": "payment_not_eligible_for_settlement"})
		return
	}
	// A single M-Pesa attempt must settle the exact outstanding sale amount in
	// this increment. Mixed/split provider tenders need an explicit allocation
	// policy and are intentionally not inferred here.
	if amount != saleTotal {
		jsonOut(w, http.StatusConflict, map[string]string{"error": "mpesa_tender_must_equal_sale_total"})
		return
	}

	rows, err := tx.Query(r.Context(), `SELECT product_id::text,sku_snapshot,name_snapshot,quantity,unit_price_minor,line_total_minor
		FROM sale_lines WHERE business_id=$1 AND sale_id=$2 ORDER BY created_at,id`, tc.BusinessID, saleID)
	if err != nil {
		catalogError(w, err)
		return
	}
	lines := make([]saleQuoteLine, 0, 8)
	for rows.Next() {
		var line saleQuoteLine
		if err = rows.Scan(&line.ProductID, &line.SKU, &line.Name, &line.Quantity, &line.UnitPrice, &line.LineTotal); err != nil {
			rows.Close()
			catalogError(w, err)
			return
		}
		lines = append(lines, line)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		catalogError(w, err)
		return
	}
	if len(lines) == 0 {
		jsonOut(w, http.StatusConflict, map[string]string{"error": "sale_lines_missing"})
		return
	}
	consumed, err := applyRecipeStockConsumption(r, tx, tc, saleID, lines)
	if err != nil {
		if errors.Is(err, errInsufficientStock) {
			jsonOut(w, http.StatusConflict, map[string]string{"error": "insufficient_stock_for_recipe"})
			return
		}
		if errors.Is(err, errRecipeConversion) {
			jsonOut(w, http.StatusUnprocessableEntity, map[string]string{"error": "recipe_unit_conversion_missing_or_inexact"})
			return
		}
		catalogError(w, err)
		return
	}

	if _, err = tx.Exec(r.Context(), `UPDATE sale_payments SET status='verified',provider_reference=$3 WHERE business_id=$1 AND id=$2 AND status='pending_verification'`, tc.BusinessID, paymentID, receipt); err != nil {
		catalogError(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE sales SET status='completed' WHERE business_id=$1 AND branch_id=$2 AND id=$3 AND status='pending_payment'`, tc.BusinessID, tc.BranchID, saleID); err != nil {
		catalogError(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE mpesa_payment_attempts SET status='verified',verified_at=now(),updated_at=now() WHERE business_id=$1 AND branch_id=$2 AND id=$3 AND status='provider_confirmed'`, tc.BusinessID, tc.BranchID, attemptID); err != nil {
		catalogError(w, err)
		return
	}
	// An order is linked only when a pending order was explicitly associated
	// with the sale by the checkout orchestration. This update is a no-op until
	// that orchestration writes source_order_id.
	var orderID string
	err = tx.QueryRow(r.Context(), `SELECT COALESCE(source_order_id::text,'') FROM sales WHERE business_id=$1 AND id=$2`, tc.BusinessID, saleID).Scan(&orderID)
	if err == nil && orderID != "" {
		tag, updateErr := tx.Exec(r.Context(), `UPDATE pos_orders SET status='checked_out',checked_out_sale_id=$3,updated_at=now() WHERE business_id=$1 AND branch_id=$2 AND id=$4 AND status IN ('open','held')`, tc.BusinessID, tc.BranchID, saleID, orderID)
		if updateErr != nil {
			catalogError(w, updateErr)
			return
		}
		if tag.RowsAffected() != 1 {
			jsonOut(w, http.StatusConflict, map[string]string{"error": "pos_order_settlement_conflict"})
			return
		}
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		catalogError(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary,metadata) VALUES($1,$2,'payments.mpesa_settled','sale',$3,'M-Pesa payment verified and sale settled',jsonb_build_object('attempt_id',$4,'receipt',$5,'amount_minor',$6,'ingredient_movements',$7))`, tc.BusinessID, tc.UserID, saleID, attemptID, receipt, amount, consumed); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, http.StatusOK, map[string]any{"attempt_id": attemptID, "sale_id": saleID, "status": "settled", "payment_status": "verified", "stock_deduction": map[string]any{"status": "recipe_backed_items_deducted", "ingredient_movements": consumed}, "idempotent_replay": false})
}

// darajaSettlementEligible is intentionally strict: provider confirmation is
// necessary but never sufficient without the matching pending tender and sale.
func darajaSettlementEligible(attemptStatus, saleStatus, paymentStatus string, amount, paymentAmount int64, receipt string) bool {
	return attemptStatus == "provider_confirmed" && saleStatus == "pending_payment" &&
		paymentStatus == "pending_verification" && strings.TrimSpace(receipt) != "" &&
		amount > 0 && amount == paymentAmount
}
