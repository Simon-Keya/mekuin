package main

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

var inventoryQuantityPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]{1,8})?$`)

type refundRequest struct {
	ApprovalToken string `json:"approval_token"`
	AmountMinor   int64  `json:"amount_minor"`
	Reason        string `json:"reason"`
}

// createSaleRefund records a manager-approved refund liability. External payment
// rail settlement is intentionally separate; this endpoint never claims money
// was returned to a card or mobile-money wallet.
func (s *server) createSaleRefund(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	saleID := chi.URLParam(r, "saleID")
	if !validUUID(saleID) {
		jsonOut(w, 400, map[string]string{"error": "invalid_sale_id"})
		return
	}
	var in refundRequest
	if !decode(w, r, &in) {
		return
	}
	in.ApprovalToken = strings.TrimSpace(in.ApprovalToken)
	in.Reason = strings.TrimSpace(in.Reason)
	if in.AmountMinor <= 0 || len(in.ApprovalToken) != 64 || len(in.Reason) < 5 || len(in.Reason) > 500 {
		jsonOut(w, 400, map[string]string{"error": "invalid_refund_request"})
		return
	}
	var status string
	var total int64
	err := tx.QueryRow(r.Context(), `SELECT status,total_minor FROM sales WHERE business_id=$1 AND branch_id=$2 AND id=$3 FOR UPDATE`, tc.BusinessID, tc.BranchID, saleID).Scan(&status, &total)
	if err == pgx.ErrNoRows {
		jsonOut(w, 404, map[string]string{"error": "sale_not_found"})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	if status != "completed" {
		jsonOut(w, 409, map[string]string{"error": "sale_not_refundable"})
		return
	}
	var refunded int64
	if err = tx.QueryRow(r.Context(), `SELECT COALESCE(sum(amount_minor),0) FROM sale_refunds WHERE business_id=$1 AND sale_id=$2`, tc.BusinessID, saleID).Scan(&refunded); err != nil {
		catalogError(w, err)
		return
	}
	if in.AmountMinor > total-refunded {
		jsonOut(w, 409, map[string]string{"error": "refund_exceeds_remaining_sale_amount"})
		return
	}
	if err = consumeManagerApproval(r.Context(), tx, tc, in.ApprovalToken, "sales.refund", saleID, &in.AmountMinor); err != nil {
		jsonOut(w, 403, map[string]string{"error": "valid_manager_approval_required"})
		return
	}
	var refundID string
	err = tx.QueryRow(r.Context(), `INSERT INTO sale_refunds(business_id,branch_id,sale_id,amount_minor,reason,refunded_by) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, tc.BusinessID, tc.BranchID, saleID, in.AmountMinor, in.Reason, tc.UserID).Scan(&refundID)
	if err != nil {
		catalogError(w, err)
		return
	}
	if err = reverseRefundInventory(r.Context(), tx, tc, saleID, refundID, refunded+in.AmountMinor, total); err != nil {
		catalogError(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary,metadata) VALUES($1,$2,'sales.refunded','sale',$3,'Manager-approved refund recorded',jsonb_build_object('refund_id',$4,'amount_minor',$5,'reason',$6))`, tc.BusinessID, tc.UserID, saleID, refundID, in.AmountMinor, in.Reason); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 201, map[string]any{"refund_id": refundID, "sale_id": saleID, "amount_minor": in.AmountMinor, "refunded_total_minor": refunded + in.AmountMinor, "sale_total_minor": total, "status": "recorded_pending_settlement"})
}

type inventoryCountLineInput struct {
	ProductID       string `json:"product_id"`
	CountedQuantity string `json:"counted_quantity"`
}
type createInventoryCountRequest struct {
	Lines     []inventoryCountLineInput `json:"lines"`
	CountMode string                    `json:"count_mode"`
	Area      string                    `json:"area"`
}
type reconcileInventoryCountRequest struct {
	ApprovalToken string `json:"approval_token"`
}

func (s *server) createInventoryCount(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in createInventoryCountRequest
	if !decode(w, r, &in) {
		return
	}
	in.CountMode = strings.TrimSpace(in.CountMode)
	in.Area = strings.TrimSpace(in.Area)
	if in.CountMode == "" {
		in.CountMode = "open"
	}
	if (in.CountMode != "open" && in.CountMode != "blind") || len(in.Area) > 120 {
		jsonOut(w, 400, map[string]string{"error": "invalid_inventory_count_mode_or_area"})
		return
	}
	if len(in.Lines) == 0 || len(in.Lines) > 500 {
		jsonOut(w, 400, map[string]string{"error": "inventory_count_requires_1_to_500_lines"})
		return
	}
	var countID string
	if err := tx.QueryRow(r.Context(), `INSERT INTO inventory_counts(business_id,branch_id,created_by,count_mode,area) VALUES($1,$2,$3,$4,$5) RETURNING id`, tc.BusinessID, tc.BranchID, tc.UserID, in.CountMode, in.Area).Scan(&countID); err != nil {
		catalogError(w, err)
		return
	}
	seen := map[string]bool{}
	for _, line := range in.Lines {
		line.ProductID = strings.ToLower(strings.TrimSpace(line.ProductID))
		line.CountedQuantity = strings.TrimSpace(line.CountedQuantity)
		if !validUUID(line.ProductID) || seen[line.ProductID] || !inventoryQuantityPattern.MatchString(line.CountedQuantity) || len(line.CountedQuantity) > 40 {
			jsonOut(w, 400, map[string]string{"error": "invalid_inventory_count_line"})
			return
		}
		seen[line.ProductID] = true
		var expected string
		err := tx.QueryRow(r.Context(), `SELECT COALESCE(sum(quantity_delta),0)::text FROM inventory_movements WHERE business_id=$1 AND branch_id=$2 AND product_id=$3`, tc.BusinessID, tc.BranchID, line.ProductID).Scan(&expected)
		if err != nil {
			catalogError(w, err)
			return
		}
		if _, err = tx.Exec(r.Context(), `INSERT INTO inventory_count_lines(business_id,count_id,product_id,expected_quantity,counted_quantity) SELECT $1,$2,p.id,$4::numeric,$5::numeric FROM products p WHERE p.business_id=$1 AND p.id=$3 AND p.is_active=true AND $5::numeric>=0`, tc.BusinessID, countID, line.ProductID, expected, line.CountedQuantity); err != nil {
			catalogError(w, err)
			return
		}
		var inserted bool
		if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM inventory_count_lines WHERE business_id=$1 AND count_id=$2 AND product_id=$3)`, tc.BusinessID, countID, line.ProductID).Scan(&inserted); err != nil {
			catalogError(w, err)
			return
		}
		if !inserted {
			jsonOut(w, 422, map[string]string{"error": "inventory_product_not_available"})
			return
		}
	}
	if _, err := tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary) VALUES($1,$2,'inventory.count.created','inventory_count',$3,'Inventory count created')`, tc.BusinessID, tc.UserID, countID); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 201, map[string]any{"count_id": countID, "status": "draft", "count_mode": in.CountMode, "area": in.Area, "line_count": len(in.Lines)})
}

func (s *server) reconcileInventoryCount(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	countID := chi.URLParam(r, "countID")
	if !validUUID(countID) {
		jsonOut(w, 400, map[string]string{"error": "invalid_count_id"})
		return
	}
	var in reconcileInventoryCountRequest
	if !decode(w, r, &in) {
		return
	}
	in.ApprovalToken = strings.TrimSpace(in.ApprovalToken)
	if len(in.ApprovalToken) != 64 {
		jsonOut(w, 400, map[string]string{"error": "approval_token_required"})
		return
	}
	var status string
	err := tx.QueryRow(r.Context(), `SELECT status FROM inventory_counts WHERE business_id=$1 AND branch_id=$2 AND id=$3 FOR UPDATE`, tc.BusinessID, tc.BranchID, countID).Scan(&status)
	if err == pgx.ErrNoRows {
		jsonOut(w, 404, map[string]string{"error": "inventory_count_not_found"})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	if status != "submitted" {
		jsonOut(w, 409, map[string]string{"error": "inventory_count_must_be_submitted_for_approval"})
		return
	}
	if err = consumeManagerApproval(r.Context(), tx, tc, in.ApprovalToken, "inventory.count.approve", countID, nil); err != nil {
		jsonOut(w, 403, map[string]string{"error": "valid_manager_approval_required"})
		return
	}
	rows, err := tx.Query(r.Context(), `SELECT product_id,expected_quantity,counted_quantity FROM inventory_count_lines WHERE business_id=$1 AND count_id=$2 ORDER BY product_id FOR UPDATE`, tc.BusinessID, countID)
	if err != nil {
		catalogError(w, err)
		return
	}
	type adjustment struct{ product string }
	changes := []adjustment{}
	for rows.Next() {
		var product, expected, counted string
		if err = rows.Scan(&product, &expected, &counted); err != nil {
			rows.Close()
			catalogError(w, err)
			return
		}
		changes = append(changes, adjustment{product: product})
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		catalogError(w, err)
		return
	}
	for _, c := range changes {
		if err = lockInventoryBalance(r.Context(), tx, tc.BusinessID, tc.BranchID, c.product); err != nil {
			catalogError(w, err)
			return
		}
		var counted, current string
		if err = tx.QueryRow(r.Context(), `SELECT counted_quantity::text FROM inventory_count_lines WHERE business_id=$1 AND count_id=$2 AND product_id=$3`, tc.BusinessID, countID, c.product).Scan(&counted); err != nil {
			catalogError(w, err)
			return
		}
		if err = tx.QueryRow(r.Context(), `SELECT COALESCE(sum(quantity_delta),0)::text FROM inventory_movements WHERE business_id=$1 AND branch_id=$2 AND product_id=$3`, tc.BusinessID, tc.BranchID, c.product).Scan(&current); err != nil {
			catalogError(w, err)
			return
		}
		if _, err = tx.Exec(r.Context(), `INSERT INTO inventory_movements(business_id,branch_id,product_id,quantity_delta,movement_type,reference_id,created_by) SELECT $1,$2,$3,($4::numeric-$5::numeric),'count_reconciliation',$6,$7 WHERE $4::numeric<>$5::numeric`, tc.BusinessID, tc.BranchID, c.product, counted, current, countID, tc.UserID); err != nil {
			catalogError(w, err)
			return
		}
	}
	tag, err := tx.Exec(r.Context(), `UPDATE inventory_counts SET status='reconciled',reconciled_by=$1,reconciled_at=now() WHERE business_id=$2 AND branch_id=$3 AND id=$4 AND status='submitted'`, tc.UserID, tc.BusinessID, tc.BranchID, countID)
	if err != nil {
		catalogError(w, err)
		return
	}
	if tag.RowsAffected() != 1 {
		jsonOut(w, 409, map[string]string{"error": "inventory_count_state_changed"})
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary,metadata) VALUES($1,$2,'inventory.count.reconciled','inventory_count',$3,'Manager-approved inventory count reconciled',jsonb_build_object('line_count',$4))`, tc.BusinessID, tc.UserID, countID, len(changes)); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"count_id": countID, "status": "reconciled", "line_count": len(changes)})
}

type updateInventoryCountRequest struct {
	Lines []inventoryCountLineInput `json:"lines"`
}

func (s *server) updateInventoryCount(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	id := chi.URLParam(r, "countID")
	if !validUUID(id) {
		jsonOut(w, 400, map[string]string{"error": "invalid_count_id"})
		return
	}
	var in updateInventoryCountRequest
	if !decode(w, r, &in) {
		return
	}
	if len(in.Lines) == 0 || len(in.Lines) > 500 {
		jsonOut(w, 400, map[string]string{"error": "inventory_count_requires_1_to_500_lines"})
		return
	}
	var status string
	if err := tx.QueryRow(r.Context(), `SELECT status FROM inventory_counts WHERE business_id=$1 AND branch_id=$2 AND id=$3 FOR UPDATE`, tc.BusinessID, tc.BranchID, id).Scan(&status); err == pgx.ErrNoRows {
		jsonOut(w, 404, map[string]string{"error": "inventory_count_not_found"})
		return
	} else if err != nil {
		catalogError(w, err)
		return
	}
	if status != "draft" {
		jsonOut(w, 409, map[string]string{"error": "only_draft_counts_can_be_edited"})
		return
	}
	seen := map[string]bool{}
	for _, line := range in.Lines {
		line.ProductID = strings.ToLower(strings.TrimSpace(line.ProductID))
		line.CountedQuantity = strings.TrimSpace(line.CountedQuantity)
		if !validUUID(line.ProductID) || seen[line.ProductID] || !inventoryQuantityPattern.MatchString(line.CountedQuantity) || len(line.CountedQuantity) > 40 {
			jsonOut(w, 400, map[string]string{"error": "invalid_inventory_count_line"})
			return
		}
		seen[line.ProductID] = true
		tag, err := tx.Exec(r.Context(), `UPDATE inventory_count_lines SET counted_quantity=$1::numeric WHERE business_id=$2 AND count_id=$3 AND product_id=$4`, line.CountedQuantity, tc.BusinessID, id, line.ProductID)
		if err != nil {
			catalogError(w, err)
			return
		}
		if tag.RowsAffected() == 0 {
			var expected string
			err = tx.QueryRow(r.Context(), `SELECT COALESCE(sum(quantity_delta),0)::text FROM inventory_movements WHERE business_id=$1 AND branch_id=$2 AND product_id=$3`, tc.BusinessID, tc.BranchID, line.ProductID).Scan(&expected)
			if err != nil {
				catalogError(w, err)
				return
			}
			tag, err = tx.Exec(r.Context(), `INSERT INTO inventory_count_lines(business_id,count_id,product_id,expected_quantity,counted_quantity) SELECT $1,$2,p.id,$4::numeric,$5::numeric FROM products p WHERE p.business_id=$1 AND p.id=$3 AND p.is_active AND $5::numeric>=0`, tc.BusinessID, id, line.ProductID, expected, line.CountedQuantity)
			if err != nil {
				catalogError(w, err)
				return
			}
			if tag.RowsAffected() != 1 {
				jsonOut(w, 422, map[string]string{"error": "inventory_product_not_available"})
				return
			}
		}
	}
	if _, err := tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary) VALUES($1,$2,'inventory.count.updated','inventory_count',$3,'Draft inventory count updated')`, tc.BusinessID, tc.UserID, id); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"count_id": id, "status": "draft", "line_count": len(in.Lines)})
}
func (s *server) submitInventoryCount(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	id := chi.URLParam(r, "countID")
	if !validUUID(id) {
		jsonOut(w, 400, map[string]string{"error": "invalid_count_id"})
		return
	}
	var status string
	err := tx.QueryRow(r.Context(), `SELECT status FROM inventory_counts WHERE business_id=$1 AND branch_id=$2 AND id=$3 FOR UPDATE`, tc.BusinessID, tc.BranchID, id).Scan(&status)
	if err == pgx.ErrNoRows {
		jsonOut(w, 404, map[string]string{"error": "inventory_count_not_found"})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	if status != "draft" {
		jsonOut(w, 409, map[string]string{"error": "only_draft_counts_can_be_submitted"})
		return
	}
	var lines int
	if err = tx.QueryRow(r.Context(), `SELECT count(*)::int FROM inventory_count_lines WHERE business_id=$1 AND count_id=$2`, tc.BusinessID, id).Scan(&lines); err != nil {
		catalogError(w, err)
		return
	}
	if lines == 0 {
		jsonOut(w, 422, map[string]string{"error": "inventory_count_has_no_lines"})
		return
	}
	tag, err := tx.Exec(r.Context(), `UPDATE inventory_counts SET status='submitted',submitted_by=$1,submitted_at=now() WHERE business_id=$2 AND branch_id=$3 AND id=$4 AND status='draft'`, tc.UserID, tc.BusinessID, tc.BranchID, id)
	if err != nil {
		catalogError(w, err)
		return
	}
	if tag.RowsAffected() != 1 {
		jsonOut(w, 409, map[string]string{"error": "inventory_count_state_changed"})
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary) VALUES($1,$2,'inventory.count.submitted','inventory_count',$3,'Inventory count submitted for manager approval')`, tc.BusinessID, tc.UserID, id); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"count_id": id, "status": "submitted", "line_count": lines})
}
