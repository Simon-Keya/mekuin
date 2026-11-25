package main

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type voidSaleRequest struct {
	ApprovalToken string `json:"approval_token"`
	Reason        string `json:"reason"`
}

// voidSale consumes the target-bound manager approval and changes the sale
// state in the same tenant transaction. Any failure rolls both changes back.
func (s *server) voidSale(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, http.StatusInternalServerError, map[string]string{"error": "tenant_context_missing"})
		return
	}
	saleID := chi.URLParam(r, "saleID")
	if !validUUID(saleID) {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "invalid_sale_id"})
		return
	}
	var in voidSaleRequest
	if !decode(w, r, &in) {
		return
	}
	in.ApprovalToken = strings.TrimSpace(in.ApprovalToken)
	in.Reason = strings.TrimSpace(in.Reason)
	if len(in.Reason) < 5 || len(in.Reason) > 500 || len(in.ApprovalToken) != 64 {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "approval_token_and_void_reason_required"})
		return
	}
	var status string
	var total int64
	err := tx.QueryRow(r.Context(), `SELECT status,total_minor FROM sales WHERE business_id=$1 AND branch_id=$2 AND id=$3 FOR UPDATE`, tc.BusinessID, tc.BranchID, saleID).Scan(&status, &total)
	if err == pgx.ErrNoRows {
		jsonOut(w, http.StatusNotFound, map[string]string{"error": "sale_not_found"})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	if status != "completed" {
		jsonOut(w, http.StatusConflict, map[string]string{"error": "sale_not_voidable"})
		return
	}
	if err = consumeManagerApproval(r.Context(), tx, tc, in.ApprovalToken, "sales.void", saleID, nil); err != nil {
		jsonOut(w, http.StatusForbidden, map[string]string{"error": "valid_manager_approval_required"})
		return
	}
	if err = reverseVoidedSaleInventory(r.Context(), tx, tc, saleID); err != nil {
		catalogError(w, err)
		return
	}
	_, err = tx.Exec(r.Context(), `UPDATE sales SET status='voided',voided_at=now(),voided_by=$1,void_reason=$2 WHERE business_id=$3 AND branch_id=$4 AND id=$5 AND status='completed'`, tc.UserID, in.Reason, tc.BusinessID, tc.BranchID, saleID)
	if err != nil {
		catalogError(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary,metadata) VALUES($1,$2,'sales.voided','sale',$3,'Sale voided with manager approval',jsonb_build_object('reason',$4,'total_minor',$5))`, tc.BusinessID, tc.UserID, saleID, in.Reason, total); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, http.StatusOK, map[string]any{"sale_id": saleID, "status": "voided", "total_minor": total})
}
