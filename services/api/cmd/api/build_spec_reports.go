package main

import (
	"net/http"
	"strings"
)

// getBuildSpecReport provides bounded, branch-scoped aggregates for the five
// report tabs. All monetary values remain in the business currency's minor unit.
func (s *server) getBuildSpecReport(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	from, to := strings.TrimSpace(r.URL.Query().Get("from")), strings.TrimSpace(r.URL.Query().Get("to"))
	if (from != "" && !validISODate(from, false)) || (to != "" && !validISODate(to, false)) || (from != "" && to != "" && from > to) {
		jsonOut(w, 400, map[string]string{"error": "invalid_date_range"})
		return
	}
	tab := strings.TrimSpace(r.URL.Query().Get("tab"))
	if tab == "" {
		tab = "sales"
	}
	if tab != "sales" && tab != "profit" && tab != "stock-value" && tab != "staff" && tab != "category-mix" {
		jsonOut(w, 400, map[string]string{"error": "invalid_report_tab"})
		return
	}
	base := map[string]any{"tab": tab, "from": from, "to": to, "currency_basis": "minor units", "scope": "selected branch"}
	switch tab {
	case "sales":
		var gross, refunds, orders int64
		var avg string
		err := tx.QueryRow(r.Context(), `SELECT COALESCE(sum(s.total_minor),0)::bigint,COALESCE(sum(COALESCE(r.amount,0)),0)::bigint,count(*)::bigint,COALESCE(avg(s.total_minor),0)::numeric(20,2)::text FROM sales s LEFT JOIN LATERAL (SELECT sum(amount_minor)::bigint amount FROM sale_refunds sr WHERE sr.business_id=s.business_id AND sr.sale_id=s.id) r ON true WHERE s.business_id=$1 AND s.branch_id=$2 AND s.status='completed' AND ($3='' OR s.created_at::date >= $3::date) AND ($4='' OR s.created_at::date <= $4::date)`, tc.BusinessID, tc.BranchID, from, to).Scan(&gross, &refunds, &orders, &avg)
		if err != nil {
			catalogError(w, err)
			return
		}
		base["gross_sales_minor"] = gross
		base["refunds_minor"] = refunds
		base["net_sales_minor"] = gross - refunds
		base["orders"] = orders
		base["average_basket_minor"] = avg
		rows, err := tx.Query(r.Context(), `SELECT s.created_at::date::text,COALESCE(sum(s.total_minor),0)::bigint FROM sales s WHERE s.business_id=$1 AND s.branch_id=$2 AND s.status='completed' AND ($3='' OR s.created_at::date >= $3::date) AND ($4='' OR s.created_at::date <= $4::date) GROUP BY s.created_at::date ORDER BY s.created_at::date DESC LIMIT 100`, tc.BusinessID, tc.BranchID, from, to)
		if err != nil {
			catalogError(w, err)
			return
		}
		daily := []map[string]any{}
		for rows.Next() {
			var d string
			var v int64
			if err = rows.Scan(&d, &v); err != nil {
				rows.Close()
				catalogError(w, err)
				return
			}
			daily = append(daily, map[string]any{"date": d, "sales_minor": v})
		}
		rows.Close()
		base["daily_sales"] = daily
	case "profit":
		var sales, refunds, cogs, expenses int64
		err := tx.QueryRow(r.Context(), `SELECT COALESCE(sum(s.total_minor),0)::bigint,COALESCE(sum(COALESCE(r.amount,0)),0)::bigint FROM sales s LEFT JOIN LATERAL(SELECT sum(amount_minor)::bigint amount FROM sale_refunds sr WHERE sr.business_id=s.business_id AND sr.sale_id=s.id) r ON true WHERE s.business_id=$1 AND s.branch_id=$2 AND s.status='completed' AND ($3='' OR s.created_at::date >= $3::date) AND ($4='' OR s.created_at::date <= $4::date)`, tc.BusinessID, tc.BranchID, from, to).Scan(&sales, &refunds)
		if err != nil {
			catalogError(w, err)
			return
		}
		err = tx.QueryRow(r.Context(), `SELECT COALESCE(sum(-vm.value_delta_minor),0)::bigint FROM inventory_valuation_movements vm JOIN inventory_movements im ON im.business_id=vm.business_id AND im.id=vm.inventory_movement_id WHERE vm.business_id=$1 AND vm.branch_id=$2 AND vm.movement_type IN ('sale_consumption','sale_void_reversal','sale_refund_reversal') AND ($3='' OR vm.created_at::date >= $3::date) AND ($4='' OR vm.created_at::date <= $4::date)`, tc.BusinessID, tc.BranchID, from, to).Scan(&cogs)
		if err != nil {
			catalogError(w, err)
			return
		}
		err = tx.QueryRow(r.Context(), `SELECT COALESCE(sum(amount_minor),0)::bigint FROM expenses WHERE business_id=$1 AND branch_id=$2 AND status IN ('approved','paid') AND currency=(SELECT currency FROM businesses WHERE id=$1) AND ($3='' OR expense_date >= $3::date) AND ($4='' OR expense_date <= $4::date)`, tc.BusinessID, tc.BranchID, from, to).Scan(&expenses)
		if err != nil {
			catalogError(w, err)
			return
		}
		net := sales - refunds
		base["net_sales_minor"] = net
		base["cost_of_goods_sold_minor"] = cogs
		base["operating_expenses_minor"] = expenses
		base["gross_profit_minor"] = net - cogs
		base["profit_after_expenses_minor"] = net - cogs - expenses
	case "stock-value":
		rows, err := tx.Query(r.Context(), `SELECT p.id,p.name,COALESCE(v.on_hand_quantity,0)::text,COALESCE(v.average_unit_cost_minor,p.cost_price_minor)::text,COALESCE(v.inventory_value_minor,0)::text FROM products p LEFT JOIN inventory_valuation_balances v ON v.business_id=p.business_id AND v.branch_id=$2 AND v.product_id=p.id WHERE p.business_id=$1 AND p.is_active ORDER BY p.name,p.id LIMIT 500`, tc.BusinessID, tc.BranchID)
		if err != nil {
			catalogError(w, err)
			return
		}
		items := []map[string]any{}
		for rows.Next() {
			var id, n, q, c, v string
			if err = rows.Scan(&id, &n, &q, &c, &v); err != nil {
				rows.Close()
				catalogError(w, err)
				return
			}
			items = append(items, map[string]any{"product_id": id, "product_name": n, "on_hand": q, "average_unit_cost_minor": c, "stock_value_minor": v})
		}
		rows.Close()
		var total string
		if err = tx.QueryRow(r.Context(), `SELECT COALESCE(sum(inventory_value_minor),0)::text FROM inventory_valuation_balances WHERE business_id=$1 AND branch_id=$2`, tc.BusinessID, tc.BranchID).Scan(&total); err != nil {
			catalogError(w, err)
			return
		}
		base["stock_value_cost_minor"] = total
		base["items"] = items
	case "staff":
		rows, err := tx.Query(r.Context(), `SELECT s.created_by::text,COALESCE(u.full_name,'Staff'),count(*)::bigint,COALESCE(sum(s.total_minor),0)::bigint FROM sales s JOIN app_users u ON u.id=s.created_by WHERE s.business_id=$1 AND s.branch_id=$2 AND s.status='completed' AND ($3='' OR s.created_at::date >= $3::date) AND ($4='' OR s.created_at::date <= $4::date) GROUP BY s.created_by,u.full_name ORDER BY sum(s.total_minor) DESC LIMIT 100`, tc.BusinessID, tc.BranchID, from, to)
		if err != nil {
			catalogError(w, err)
			return
		}
		items := []map[string]any{}
		for rows.Next() {
			var id, n string
			var count, total int64
			if err = rows.Scan(&id, &n, &count, &total); err != nil {
				rows.Close()
				catalogError(w, err)
				return
			}
			items = append(items, map[string]any{"user_id": id, "staff_name": n, "orders": count, "sales_minor": total})
		}
		rows.Close()
		base["items"] = items
	case "category-mix":
		rows, err := tx.Query(r.Context(), `SELECT COALESCE(pc.name,'Uncategorised'),COALESCE(sum(sl.line_total_minor),0)::bigint FROM sales s JOIN sale_lines sl ON sl.business_id=s.business_id AND sl.sale_id=s.id JOIN products p ON p.business_id=sl.business_id AND p.id=sl.product_id LEFT JOIN product_categories pc ON pc.business_id=p.business_id AND pc.id=p.category_id WHERE s.business_id=$1 AND s.branch_id=$2 AND s.status='completed' AND ($3='' OR s.created_at::date >= $3::date) AND ($4='' OR s.created_at::date <= $4::date) GROUP BY pc.name ORDER BY sum(sl.line_total_minor) DESC LIMIT 100`, tc.BusinessID, tc.BranchID, from, to)
		if err != nil {
			catalogError(w, err)
			return
		}
		items := []map[string]any{}
		for rows.Next() {
			var n string
			var total int64
			if err = rows.Scan(&n, &total); err != nil {
				rows.Close()
				catalogError(w, err)
				return
			}
			items = append(items, map[string]any{"category": n, "sales_minor": total})
		}
		rows.Close()
		base["items"] = items
	}
	jsonOut(w, 200, base)
}
