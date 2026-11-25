package main

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
)

var inventoryDeltaPattern = regexp.MustCompile(`^-?[0-9]+(?:\.[0-9]{1,8})?$`)

type manualInventoryAdjustmentRequest struct {
	ProductID string `json:"product_id"`
	Delta     string `json:"quantity_delta"`
	Reason    string `json:"reason"`
}

// listInventoryMovements exposes the immutable movement ledger, scoped to the
// active tenant and branch. Product names are joined only within that tenant.
func (s *server) listInventoryMovements(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, http.StatusInternalServerError, map[string]string{"error": "tenant_context_missing"})
		return
	}
	limit, offset, q, valid := listBounds(r)
	if !valid {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "invalid_pagination"})
		return
	}
	rows, err := tx.Query(r.Context(), `SELECT m.id,m.product_id,p.name,m.quantity_delta::text,m.movement_type,m.reference_id,m.created_at
		FROM inventory_movements m JOIN products p ON p.business_id=m.business_id AND p.id=m.product_id
		WHERE m.business_id=$1 AND m.branch_id=$2 AND ($3='' OR p.name ILIKE '%'||$3||'%' OR m.movement_type ILIKE '%'||$3||'%')
		ORDER BY m.created_at DESC,m.id DESC LIMIT $4 OFFSET $5`, tc.BusinessID, tc.BranchID, q, limit, offset)
	if err != nil {
		catalogError(w, err)
		return
	}
	defer rows.Close()
	type movement struct {
		ID            string  `json:"id"`
		ProductID     string  `json:"product_id"`
		ProductName   string  `json:"product_name"`
		QuantityDelta string  `json:"quantity_delta"`
		MovementType  string  `json:"movement_type"`
		ReferenceID   *string `json:"reference_id,omitempty"`
		CreatedAt     string  `json:"created_at"`
	}
	items := []movement{}
	for rows.Next() {
		var item movement
		if err = rows.Scan(&item.ID, &item.ProductID, &item.ProductName, &item.QuantityDelta, &item.MovementType, &item.ReferenceID, &item.CreatedAt); err != nil {
			catalogError(w, err)
			return
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, http.StatusOK, map[string]any{"items": items, "limit": limit, "offset": offset})
}

// adjustInventory records a reasoned manual quantity delta in the append-only
// ledger. A negative adjustment cannot take the branch below zero stock.
func (s *server) adjustInventory(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in manualInventoryAdjustmentRequest
	if !decode(w, r, &in) {
		return
	}
	in.ProductID = strings.ToLower(strings.TrimSpace(in.ProductID))
	in.Delta = strings.TrimSpace(in.Delta)
	in.Reason = strings.TrimSpace(in.Reason)
	if !validUUID(in.ProductID) || !inventoryDeltaPattern.MatchString(in.Delta) || len(in.Delta) > 40 || strings.Trim(in.Delta, "-0.") == "" || len(in.Reason) < 5 || len(in.Reason) > 300 {
		jsonOut(w, 400, map[string]string{"error": "invalid_inventory_adjustment"})
		return
	}
	var productID string
	err := tx.QueryRow(r.Context(), `SELECT id FROM products WHERE business_id=$1 AND id=$2 AND is_active`, tc.BusinessID, in.ProductID).Scan(&productID)
	if err == pgx.ErrNoRows {
		jsonOut(w, 404, map[string]string{"error": "inventory_product_not_found"})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	if err = lockInventoryBalance(r.Context(), tx, tc.BusinessID, tc.BranchID, in.ProductID); err != nil {
		catalogError(w, err)
		return
	}
	var onHand string
	if err = tx.QueryRow(r.Context(), `SELECT COALESCE(sum(quantity_delta),0)::text FROM inventory_movements WHERE business_id=$1 AND branch_id=$2 AND product_id=$3`, tc.BusinessID, tc.BranchID, in.ProductID).Scan(&onHand); err != nil {
		catalogError(w, err)
		return
	}
	var valid bool
	if err = tx.QueryRow(r.Context(), `SELECT $1::numeric<>0 AND $1::numeric+$2::numeric>=0`, in.Delta, onHand).Scan(&valid); err != nil {
		catalogError(w, err)
		return
	}
	if !valid {
		jsonOut(w, 409, map[string]string{"error": "adjustment_would_create_invalid_stock_balance"})
		return
	}
	var movementID string
	err = tx.QueryRow(r.Context(), `INSERT INTO inventory_movements(business_id,branch_id,product_id,quantity_delta,movement_type,created_by) VALUES($1,$2,$3,$4::numeric,'manual_adjustment',$5) RETURNING id`, tc.BusinessID, tc.BranchID, in.ProductID, in.Delta, tc.UserID).Scan(&movementID)
	if err != nil {
		catalogError(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary,metadata) VALUES($1,$2,'inventory.adjusted','inventory_movement',$3,'Manual inventory adjustment recorded',jsonb_build_object('product_id',$4,'quantity_delta',$5,'reason',$6))`, tc.BusinessID, tc.UserID, movementID, in.ProductID, in.Delta, in.Reason); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 201, map[string]any{"movement_id": movementID, "product_id": in.ProductID, "quantity_delta": in.Delta, "previous_on_hand": onHand, "status": "recorded"})
}

func (s *server) listInventoryBranches(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	rows, err := tx.Query(r.Context(), `SELECT b.id,b.name,b.is_active FROM branches b
		JOIN membership_branches mb ON mb.business_id=b.business_id AND mb.branch_id=b.id
		JOIN memberships m ON m.business_id=mb.business_id AND m.id=mb.membership_id
		WHERE b.business_id=$1 AND m.user_id=$2 AND m.status='active' AND b.is_active
		ORDER BY b.name,b.id`, tc.BusinessID, tc.UserID)
	if err != nil {
		catalogError(w, err)
		return
	}
	defer rows.Close()
	type branch struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		IsActive bool   `json:"is_active"`
	}
	items := []branch{}
	for rows.Next() {
		var v branch
		if err = rows.Scan(&v.ID, &v.Name, &v.IsActive); err != nil {
			catalogError(w, err)
			return
		}
		items = append(items, v)
	}
	if err = rows.Err(); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, items)
}

func (s *server) getInventoryValuation(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var stockValue, lowStock, outOfStock, wastageValue string
	err := tx.QueryRow(r.Context(), `SELECT COALESCE(sum(v.inventory_value_minor),0)::text,
  count(*) FILTER (WHERE v.on_hand_quantity>0 AND v.on_hand_quantity<=p.reorder_level)::text,
  count(*) FILTER (WHERE v.on_hand_quantity<=0)::text,
  COALESCE((SELECT sum(abs(vm.value_delta_minor)) FROM inventory_valuation_movements vm WHERE vm.business_id=$1 AND vm.branch_id=$2 AND vm.movement_type='wastage' AND vm.created_at>=date_trunc('month',now())),0)::text
  FROM products p LEFT JOIN inventory_valuation_balances v ON v.business_id=p.business_id AND v.branch_id=$2 AND v.product_id=p.id
  WHERE p.business_id=$1 AND p.is_active`, tc.BusinessID, tc.BranchID).Scan(&stockValue, &lowStock, &outOfStock, &wastageValue)
	if err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"branch_id": tc.BranchID, "stock_value_cost_minor": stockValue, "low_stock_products": lowStock, "out_of_stock_products": outOfStock, "wastage_month_to_date_minor": wastageValue, "valuation_method": "per-branch moving weighted-average cost", "currency_basis": "minor units"})
}
