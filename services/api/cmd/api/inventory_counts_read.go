package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func (s *server) listInventoryCounts(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	limit, offset, _, valid := listBounds(r)
	if !valid {
		jsonOut(w, 400, map[string]string{"error": "invalid_pagination"})
		return
	}
	rows, err := tx.Query(r.Context(), `SELECT c.id,c.status,c.count_mode,c.area,c.created_at,count(l.id)::int
		FROM inventory_counts c LEFT JOIN inventory_count_lines l ON l.business_id=c.business_id AND l.count_id=c.id
		WHERE c.business_id=$1 AND c.branch_id=$2
		GROUP BY c.id ORDER BY c.created_at DESC,c.id DESC LIMIT $3 OFFSET $4`, tc.BusinessID, tc.BranchID, limit, offset)
	if err != nil {
		catalogError(w, err)
		return
	}
	defer rows.Close()
	type item struct {
		ID        string `json:"id"`
		Status    string `json:"status"`
		CountMode string `json:"count_mode"`
		Area      string `json:"area"`
		CreatedAt string `json:"created_at"`
		LineCount int    `json:"line_count"`
	}
	items := []item{}
	for rows.Next() {
		var v item
		if err = rows.Scan(&v.ID, &v.Status, &v.CountMode, &v.Area, &v.CreatedAt, &v.LineCount); err != nil {
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

func (s *server) getInventoryCount(w http.ResponseWriter, r *http.Request) {
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
	var status, mode, area, createdAt string
	err := tx.QueryRow(r.Context(), `SELECT status,count_mode,area,created_at::text FROM inventory_counts WHERE business_id=$1 AND branch_id=$2 AND id=$3`, tc.BusinessID, tc.BranchID, id).Scan(&status, &mode, &area, &createdAt)
	if err == pgx.ErrNoRows {
		jsonOut(w, 404, map[string]string{"error": "inventory_count_not_found"})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	rows, err := tx.Query(r.Context(), `SELECT l.product_id,p.name,CASE WHEN $3='blind' AND $4<>'reconciled' THEN NULL ELSE l.expected_quantity::text END,l.counted_quantity::text,
		CASE WHEN $3='blind' AND $4<>'reconciled' THEN NULL ELSE (l.counted_quantity-l.expected_quantity)::text END,
		CASE WHEN $3='blind' AND $4<>'reconciled' THEN NULL ELSE round((l.counted_quantity-l.expected_quantity)*CASE WHEN p.product_kind='liquid' THEN COALESCE(v.average_unit_cost_minor,p.cost_price_minor)::numeric/NULLIF(p.bottle_size,0) ELSE COALESCE(v.average_unit_cost_minor,p.cost_price_minor)::numeric END)::text END,
		CASE WHEN $3='blind' AND $4<>'reconciled' THEN NULL ELSE round((l.counted_quantity-l.expected_quantity)*CASE WHEN p.product_kind='liquid' THEN p.sale_price_minor::numeric/NULLIF(p.serving_size,0) ELSE p.sale_price_minor::numeric END)::text END
		FROM inventory_count_lines l JOIN products p ON p.business_id=l.business_id AND p.id=l.product_id
		LEFT JOIN inventory_valuation_balances v ON v.business_id=l.business_id AND v.branch_id=$5 AND v.product_id=l.product_id
		WHERE l.business_id=$1 AND l.count_id=$2 ORDER BY p.name,p.id`, tc.BusinessID, id, mode, status, tc.BranchID)
	if err != nil {
		catalogError(w, err)
		return
	}
	defer rows.Close()
	type line struct {
		ProductID    string  `json:"product_id"`
		ProductName  string  `json:"product_name"`
		Expected     *string `json:"expected_quantity,omitempty"`
		Counted      string  `json:"counted_quantity"`
		Variance     *string `json:"variance,omitempty"`
		CostImpact   *string `json:"cost_impact_minor,omitempty"`
		RetailImpact *string `json:"retail_impact_minor,omitempty"`
	}
	lines := []line{}
	for rows.Next() {
		var v line
		if err = rows.Scan(&v.ProductID, &v.ProductName, &v.Expected, &v.Counted, &v.Variance, &v.CostImpact, &v.RetailImpact); err != nil {
			catalogError(w, err)
			return
		}
		lines = append(lines, v)
	}
	if err = rows.Err(); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"id": id, "status": status, "count_mode": mode, "area": area, "created_at": createdAt, "lines": lines, "line_count": len(lines), "expected_values_visible": mode != "blind" || status == "reconciled"})
}
