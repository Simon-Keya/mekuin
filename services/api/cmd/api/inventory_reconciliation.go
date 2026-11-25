package main

import (
	"net/http"
	"time"
)

func (s *server) getInventoryReconciliation(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	fromDate, err := time.Parse("2006-01-02", from)
	if err != nil {
		jsonOut(w, 400, map[string]string{"error": "valid_from_date_required"})
		return
	}
	toDate, err := time.Parse("2006-01-02", to)
	if err != nil || toDate.Before(fromDate) || toDate.Sub(fromDate) > 366*24*time.Hour {
		jsonOut(w, 400, map[string]string{"error": "invalid_reconciliation_date_range"})
		return
	}
	rows, err := tx.Query(r.Context(), `WITH opening AS (
	 SELECT DISTINCT ON (l.product_id) l.product_id,l.counted_quantity
	 FROM inventory_counts c JOIN inventory_count_lines l ON l.business_id=c.business_id AND l.count_id=c.id
	 WHERE c.business_id=$1 AND c.branch_id=$2 AND c.status='reconciled' AND COALESCE(c.reconciled_at,c.created_at) < $3::date
	 ORDER BY l.product_id,COALESCE(c.reconciled_at,c.created_at) DESC,c.id DESC
	), closing AS (
	 SELECT DISTINCT ON (l.product_id) l.product_id,l.counted_quantity
	 FROM inventory_counts c JOIN inventory_count_lines l ON l.business_id=c.business_id AND l.count_id=c.id
	 WHERE c.business_id=$1 AND c.branch_id=$2 AND c.status='reconciled' AND COALESCE(c.reconciled_at,c.created_at) >= $3::date AND COALESCE(c.reconciled_at,c.created_at) < ($4::date + interval '1 day')
	 ORDER BY l.product_id,COALESCE(c.reconciled_at,c.created_at) DESC,c.id DESC
	), purchases AS (
	 SELECT rl.product_id,sum(rl.received_quantity) quantity
	 FROM purchase_receipts pr JOIN purchase_receipt_lines rl ON rl.business_id=pr.business_id AND rl.receipt_id=pr.id
	 WHERE pr.business_id=$1 AND pr.branch_id=$2 AND pr.created_at >= $3::date AND pr.created_at < ($4::date + interval '1 day')
	 GROUP BY rl.product_id
	), theoretical AS (
	 SELECT snap.ingredient_product_id product_id,
	        sum(snap.consumed_quantity * CASE WHEN s.status='voided' THEN 0 ELSE greatest(0,1-COALESCE(refunds.refunded_minor,0)::numeric/NULLIF(s.total_minor,0)) END) quantity
	 FROM sale_recipe_consumption_snapshots snap
	 JOIN sales s ON s.business_id=snap.business_id AND s.branch_id=snap.branch_id AND s.id=snap.sale_id
	 LEFT JOIN LATERAL (SELECT sum(amount_minor)::numeric refunded_minor FROM sale_refunds rf WHERE rf.business_id=s.business_id AND rf.sale_id=s.id) refunds ON true
	 WHERE snap.business_id=$1 AND snap.branch_id=$2 AND s.created_at >= $3::date AND s.created_at < ($4::date + interval '1 day')
	 GROUP BY snap.ingredient_product_id
	)
	SELECT p.id,p.name,p.product_kind,COALESCE(o.counted_quantity,0)::text,
	       COALESCE(pr.quantity,0)::text,COALESCE(t.quantity,0)::text,
	       COALESCE(c.counted_quantity,0)::text,
	       (o.counted_quantity IS NOT NULL) AS has_opening,(c.counted_quantity IS NOT NULL) AS has_closing,
	       CASE WHEN o.counted_quantity IS NULL OR c.counted_quantity IS NULL THEN NULL ELSE (o.counted_quantity+COALESCE(pr.quantity,0)-c.counted_quantity)::text END,
	       CASE WHEN o.counted_quantity IS NULL OR c.counted_quantity IS NULL THEN NULL ELSE (COALESCE(t.quantity,0)-(o.counted_quantity+COALESCE(pr.quantity,0)-c.counted_quantity))::text END,
	       CASE WHEN o.counted_quantity IS NULL OR c.counted_quantity IS NULL THEN NULL ELSE round(greatest((o.counted_quantity+COALESCE(pr.quantity,0)-c.counted_quantity)-COALESCE(t.quantity,0),0)*CASE WHEN p.product_kind='liquid' THEN COALESCE(v.average_unit_cost_minor,p.cost_price_minor)::numeric/NULLIF(p.bottle_size,0) ELSE COALESCE(v.average_unit_cost_minor,p.cost_price_minor)::numeric END)::text END
	FROM products p LEFT JOIN inventory_valuation_balances v ON v.business_id=p.business_id AND v.branch_id=$2 AND v.product_id=p.id LEFT JOIN opening o ON o.product_id=p.id LEFT JOIN closing c ON c.product_id=p.id LEFT JOIN purchases pr ON pr.product_id=p.id LEFT JOIN theoretical t ON t.product_id=p.id
	WHERE p.business_id=$1 AND p.is_active AND (o.product_id IS NOT NULL OR c.product_id IS NOT NULL OR pr.product_id IS NOT NULL OR t.product_id IS NOT NULL)
	ORDER BY p.name,p.id`, tc.BusinessID, tc.BranchID, fromDate.Format("2006-01-02"), toDate.Format("2006-01-02"))
	if err != nil {
		catalogError(w, err)
		return
	}
	defer rows.Close()
	type item struct {
		ProductID         string  `json:"product_id"`
		ProductName       string  `json:"product_name"`
		ProductKind       string  `json:"product_kind"`
		OpeningCount      string  `json:"opening_count"`
		Purchases         string  `json:"purchases"`
		TheoreticalUse    string  `json:"theoretical_use"`
		ClosingCount      string  `json:"closing_count"`
		HasOpening        bool    `json:"has_opening_count"`
		HasClosing        bool    `json:"has_closing_count"`
		ActualUse         *string `json:"actual_use,omitempty"`
		Variance          *string `json:"variance,omitempty"`
		LeakageValueMinor *string `json:"leakage_value_minor,omitempty"`
	}
	items := []item{}
	for rows.Next() {
		var v item
		if err = rows.Scan(&v.ProductID, &v.ProductName, &v.ProductKind, &v.OpeningCount, &v.Purchases, &v.TheoreticalUse, &v.ClosingCount, &v.HasOpening, &v.HasClosing, &v.ActualUse, &v.Variance, &v.LeakageValueMinor); err != nil {
			catalogError(w, err)
			return
		}
		items = append(items, v)
	}
	if err = rows.Err(); err != nil {
		catalogError(w, err)
		return
	}
	var salesWithRecipeMovements, salesWithSnapshots int64
	if err = tx.QueryRow(r.Context(), `SELECT count(DISTINCT im.reference_id),count(DISTINCT snap.sale_id) FROM inventory_movements im LEFT JOIN sale_recipe_consumption_snapshots snap ON snap.business_id=im.business_id AND snap.branch_id=im.branch_id AND snap.sale_id=im.reference_id WHERE im.business_id=$1 AND im.branch_id=$2 AND im.movement_type='sale_consumption' AND im.created_at >= $3::date AND im.created_at < ($4::date + interval '1 day')`, tc.BusinessID, tc.BranchID, from, to).Scan(&salesWithRecipeMovements, &salesWithSnapshots); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"from": from, "to": to, "items": items, "count": len(items), "theoretical_basis": "immutable checkout-time SaleItem × recipe ingredient/direct-stock snapshots, adjusted for voids and proportional refunds", "actual_basis": "opening reconciled physical count + posted purchase receipts - closing reconciled physical count", "recipe_snapshot_coverage": map[string]any{"sales_with_recipe_consumption": salesWithRecipeMovements, "sales_with_historical_recipe_snapshots": salesWithSnapshots, "complete": salesWithRecipeMovements == salesWithSnapshots}, "missing_count_baseline": "Actual use and leakage are withheld until both opening and closing counts exist."})
}
