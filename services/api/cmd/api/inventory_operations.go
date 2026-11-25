package main

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type inventoryLossLine struct {
	ProductID string `json:"product_id"`
	Quantity  string `json:"quantity"`
}
type inventoryLossRequest struct {
	Reason string              `json:"reason"`
	Lines  []inventoryLossLine `json:"lines"`
}
type inventoryTransferRequest struct {
	DestinationBranchID string              `json:"destination_branch_id"`
	Reason              string              `json:"reason"`
	Lines               []inventoryLossLine `json:"lines"`
}

func lockInventoryBalance(ctx context.Context, tx pgx.Tx, businessID, branchID, productID string) error {
	key := businessID + ":" + branchID + ":stock:" + productID
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, key)
	return err
}

// listInventoryBalances derives on-hand quantities from the immutable movement
// ledger. No separately writable balance is introduced in this phase.
func (s *server) listInventoryBalances(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	limit, offset, q, valid := listBounds(r)
	if !valid {
		jsonOut(w, 400, map[string]string{"error": "invalid_pagination"})
		return
	}
	rows, err := tx.Query(r.Context(), `SELECT p.id,p.name,p.sku,p.base_unit_id,p.reorder_level::text,COALESCE(v.on_hand_quantity,sum(m.quantity_delta),0)::text AS on_hand,COALESCE(v.average_unit_cost_minor,p.cost_price_minor)::text AS average_unit_cost_minor,COALESCE(v.inventory_value_minor,0)::text AS inventory_value_minor
		FROM products p LEFT JOIN inventory_movements m ON m.business_id=p.business_id AND m.branch_id=$2 AND m.product_id=p.id
		LEFT JOIN inventory_valuation_balances v ON v.business_id=p.business_id AND v.branch_id=$2 AND v.product_id=p.id
		WHERE p.business_id=$1 AND p.is_active AND ($3='' OR p.name ILIKE '%'||$3||'%' OR p.sku ILIKE '%'||$3||'%')
		GROUP BY p.id,v.on_hand_quantity,v.average_unit_cost_minor,v.inventory_value_minor ORDER BY p.name,p.id LIMIT $4 OFFSET $5`, tc.BusinessID, tc.BranchID, q, limit, offset)
	if err != nil {
		catalogError(w, err)
		return
	}
	defer rows.Close()
	type item struct {
		ProductID            string  `json:"product_id"`
		Name                 string  `json:"name"`
		SKU                  *string `json:"sku,omitempty"`
		BaseUnitID           string  `json:"base_unit_id"`
		ReorderLevel         string  `json:"reorder_level"`
		OnHand               string  `json:"on_hand"`
		AverageUnitCostMinor string  `json:"average_unit_cost_minor"`
		InventoryValueMinor  string  `json:"inventory_value_minor"`
	}
	items := []item{}
	for rows.Next() {
		var v item
		if err = rows.Scan(&v.ProductID, &v.Name, &v.SKU, &v.BaseUnitID, &v.ReorderLevel, &v.OnHand, &v.AverageUnitCostMinor, &v.InventoryValueMinor); err != nil {
			catalogError(w, err)
			return
		}
		items = append(items, v)
	}
	if err = rows.Err(); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"items": items, "limit": limit, "offset": offset})
}

func (s *server) recordInventoryLoss(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	kind := chi.URLParam(r, "kind")
	if kind != "wastage" && kind != "breakage" {
		jsonOut(w, 404, map[string]string{"error": "inventory_loss_type_not_found"})
		return
	}
	var in inventoryLossRequest
	if !decode(w, r, &in) {
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if len(in.Lines) == 0 || len(in.Lines) > 100 || len(in.Reason) < 5 || len(in.Reason) > 300 {
		jsonOut(w, 400, map[string]string{"error": "invalid_inventory_loss_request"})
		return
	}
	sort.Slice(in.Lines, func(i, j int) bool { return in.Lines[i].ProductID < in.Lines[j].ProductID })
	var adjustmentID string
	if err := tx.QueryRow(r.Context(), `SELECT gen_random_uuid()::text`).Scan(&adjustmentID); err != nil {
		catalogError(w, err)
		return
	}
	seen := map[string]bool{}
	for _, line := range in.Lines {
		id := strings.ToLower(strings.TrimSpace(line.ProductID))
		qty := strings.TrimSpace(line.Quantity)
		if !validUUID(id) || seen[id] || !inventoryQuantityPattern.MatchString(qty) || len(qty) > 40 || strings.Trim(qty, "0.") == "" || strings.HasPrefix(qty, "-") {
			jsonOut(w, 400, map[string]string{"error": "invalid_inventory_loss_line"})
			return
		}
		seen[id] = true
		var productID string
		if err := lockInventoryBalance(r.Context(), tx, tc.BusinessID, tc.BranchID, id); err != nil {
			catalogError(w, err)
			return
		}
		err := tx.QueryRow(r.Context(), `SELECT id FROM products WHERE business_id=$1 AND id=$2 AND is_active`, tc.BusinessID, id).Scan(&productID)
		if err == pgx.ErrNoRows {
			jsonOut(w, 404, map[string]string{"error": "inventory_product_not_found"})
			return
		}
		if err != nil {
			catalogError(w, err)
			return
		}
		var onHand string
		if err = tx.QueryRow(r.Context(), `SELECT COALESCE(sum(quantity_delta),0)::text FROM inventory_movements WHERE business_id=$1 AND branch_id=$2 AND product_id=$3`, tc.BusinessID, tc.BranchID, id).Scan(&onHand); err != nil {
			catalogError(w, err)
			return
		}
		var enough bool
		if err = tx.QueryRow(r.Context(), `SELECT $1::numeric <= $2::numeric`, qty, onHand).Scan(&enough); err != nil {
			catalogError(w, err)
			return
		}
		if !enough {
			jsonOut(w, 409, map[string]string{"error": "insufficient_stock"})
			return
		}
		if _, err = tx.Exec(r.Context(), `INSERT INTO inventory_movements(business_id,branch_id,product_id,quantity_delta,movement_type,reference_id,created_by) VALUES($1,$2,$3,-$4::numeric,$5,$6,$7)`, tc.BusinessID, tc.BranchID, id, qty, kind, adjustmentID, tc.UserID); err != nil {
			catalogError(w, err)
			return
		}
	}
	if _, err := tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary,metadata) VALUES($1,$2,$3,'inventory_adjustment',$4,'Inventory loss recorded',jsonb_build_object('line_count',$5,'reason',$6))`, tc.BusinessID, tc.UserID, "inventory."+kind, adjustmentID, len(in.Lines), in.Reason); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 201, map[string]any{"adjustment_id": adjustmentID, "status": "recorded", "movement_type": kind, "line_count": len(in.Lines)})
}

func (s *server) transferInventory(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in inventoryTransferRequest
	if !decode(w, r, &in) {
		return
	}
	in.DestinationBranchID = strings.ToLower(strings.TrimSpace(in.DestinationBranchID))
	in.Reason = strings.TrimSpace(in.Reason)
	if !validUUID(in.DestinationBranchID) || in.DestinationBranchID == tc.BranchID || len(in.Lines) == 0 || len(in.Lines) > 100 || len(in.Reason) < 5 || len(in.Reason) > 300 {
		jsonOut(w, 400, map[string]string{"error": "invalid_inventory_transfer"})
		return
	}
	var branchID string
	err := tx.QueryRow(r.Context(), `SELECT b.id FROM branches b JOIN membership_branches mb ON mb.business_id=b.business_id AND mb.branch_id=b.id JOIN memberships m ON m.business_id=mb.business_id AND m.id=mb.membership_id WHERE b.business_id=$1 AND b.id=$2 AND b.is_active AND m.user_id=$3 AND m.status='active'`, tc.BusinessID, in.DestinationBranchID, tc.UserID).Scan(&branchID)
	if err == pgx.ErrNoRows {
		jsonOut(w, 403, map[string]string{"error": "destination_branch_not_assigned"})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	var transferID string
	if err = tx.QueryRow(r.Context(), `SELECT gen_random_uuid()::text`).Scan(&transferID); err != nil {
		catalogError(w, err)
		return
	}
	sort.Slice(in.Lines, func(i, j int) bool { return in.Lines[i].ProductID < in.Lines[j].ProductID })
	seen := map[string]bool{}
	for _, line := range in.Lines {
		id := strings.ToLower(strings.TrimSpace(line.ProductID))
		qty := strings.TrimSpace(line.Quantity)
		if !validUUID(id) || seen[id] || !inventoryQuantityPattern.MatchString(qty) || len(qty) > 40 || strings.Trim(qty, "0.") == "" {
			jsonOut(w, 400, map[string]string{"error": "invalid_inventory_transfer_line"})
			return
		}
		seen[id] = true
		var pid string
		branches := []string{tc.BranchID, in.DestinationBranchID}
		sort.Strings(branches)
		for _, branchID := range branches {
			if err = lockInventoryBalance(r.Context(), tx, tc.BusinessID, branchID, id); err != nil {
				catalogError(w, err)
				return
			}
		}
		err = tx.QueryRow(r.Context(), `SELECT id FROM products WHERE business_id=$1 AND id=$2 AND is_active`, tc.BusinessID, id).Scan(&pid)
		if err == pgx.ErrNoRows {
			jsonOut(w, 404, map[string]string{"error": "inventory_product_not_found"})
			return
		}
		if err != nil {
			catalogError(w, err)
			return
		}
		var onHand string
		if err = tx.QueryRow(r.Context(), `SELECT COALESCE(sum(quantity_delta),0)::text FROM inventory_movements WHERE business_id=$1 AND branch_id=$2 AND product_id=$3`, tc.BusinessID, tc.BranchID, id).Scan(&onHand); err != nil {
			catalogError(w, err)
			return
		}
		var enough bool
		if err = tx.QueryRow(r.Context(), `SELECT $1::numeric <= $2::numeric`, qty, onHand).Scan(&enough); err != nil {
			catalogError(w, err)
			return
		}
		if !enough {
			jsonOut(w, 409, map[string]string{"error": "insufficient_stock"})
			return
		}
		for _, entry := range []struct{ branch, delta, typ string }{{tc.BranchID, "-", "transfer_out"}, {in.DestinationBranchID, "+", "transfer_in"}} {
			sign := entry.delta
			if _, err = tx.Exec(r.Context(), `INSERT INTO inventory_movements(business_id,branch_id,product_id,quantity_delta,movement_type,reference_id,created_by) VALUES($1,$2,$3,CASE WHEN $4='-' THEN -$5::numeric ELSE $5::numeric END,$6,$7,$8)`, tc.BusinessID, entry.branch, id, sign, qty, entry.typ, transferID, tc.UserID); err != nil {
				catalogError(w, err)
				return
			}
		}
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary,metadata) VALUES($1,$2,'inventory.transferred','inventory_transfer',$3,'Stock transferred between assigned branches',jsonb_build_object('destination_branch_id',$4,'line_count',$5,'reason',$6))`, tc.BusinessID, tc.UserID, transferID, in.DestinationBranchID, len(in.Lines), in.Reason); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 201, map[string]any{"transfer_id": transferID, "status": "completed", "line_count": len(in.Lines)})
}
