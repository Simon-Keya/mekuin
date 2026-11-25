package main

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// listCashShiftHistory returns closed shifts for the selected branch. Staff can
// see their own shifts; managers can see all shifts in the branch.
func (s *server) listCashShiftHistory(w http.ResponseWriter, r *http.Request) {
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
	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 1 && n <= 100 {
			limit = n
		} else {
			jsonOut(w, 400, map[string]string{"error": "invalid_limit"})
			return
		}
	}
	rows, err := tx.Query(r.Context(), `
		SELECT cs.id::text,cs.user_id::text,cs.status,cs.opening_float_minor,
			COALESCE(cs.expected_cash_minor,0),COALESCE(cs.counted_cash_minor,0),COALESCE(cs.variance_minor,0),
			cs.opened_at,cs.closed_at,
			COALESCE((SELECT sum(p.amount_minor) FROM sale_payments p JOIN sales s ON s.business_id=p.business_id AND s.id=p.sale_id AND s.branch_id=p.branch_id WHERE p.business_id=cs.business_id AND p.branch_id=cs.branch_id AND p.cash_shift_id=cs.id AND p.method='cash' AND p.status IN ('recorded','verified') AND s.status='completed'),0)::bigint,
			COALESCE((SELECT sum(m.amount_minor) FROM cash_shift_movements m WHERE m.business_id=cs.business_id AND m.shift_id=cs.id AND m.direction='paid_in'),0)::bigint,
			COALESCE((SELECT sum(m.amount_minor) FROM cash_shift_movements m WHERE m.business_id=cs.business_id AND m.shift_id=cs.id AND m.direction='paid_out'),0)::bigint
		FROM cash_shifts cs WHERE cs.business_id=$1 AND cs.branch_id=$2 AND cs.status='closed'
		AND ($3='' OR cs.closed_at::date >= $3::date) AND ($4='' OR cs.closed_at::date <= $4::date)
		AND ($5::boolean OR cs.user_id=$6::uuid)
		ORDER BY cs.closed_at DESC,cs.id DESC LIMIT $7`, tc.BusinessID, tc.BranchID, from, to, hasPermission(tc.Role, "shifts.manage"), tc.UserID, limit)
	if err != nil {
		catalogError(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, userID, status string
		var opening, expected, counted, variance, sales, paidIn, paidOut int64
		var openedAt time.Time
		var closedAt time.Time
		if err = rows.Scan(&id, &userID, &status, &opening, &expected, &counted, &variance, &openedAt, &closedAt, &sales, &paidIn, &paidOut); err != nil {
			catalogError(w, err)
			return
		}
		items = append(items, map[string]any{"shift_id": id, "user_id": userID, "status": status, "opening_float_minor": opening, "cash_sales_minor": sales, "paid_in_minor": paidIn, "paid_out_minor": paidOut, "expected_cash_minor": expected, "counted_cash_minor": counted, "variance_minor": variance, "opened_at": openedAt, "closed_at": closedAt})
	}
	if err = rows.Err(); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"items": items, "from": from, "to": to, "limit": limit, "scope": map[bool]string{true: "branch", false: "own"}[hasPermission(tc.Role, "shifts.manage")]})
}

// getOperationalReportSummary combines independent currency totals without
// converting or adding unlike currencies.
func (s *server) getOperationalReportSummary(w http.ResponseWriter, r *http.Request) {
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
	rows, err := tx.Query(r.Context(), `SELECT currency,status,count(*)::bigint,sum(amount_minor)::bigint FROM expenses WHERE business_id=$1 AND branch_id=$2 AND ($3='' OR expense_date >= $3::date) AND ($4='' OR expense_date <= $4::date) GROUP BY currency,status ORDER BY currency,status`, tc.BusinessID, tc.BranchID, from, to)
	if err != nil {
		catalogError(w, err)
		return
	}
	defer rows.Close()
	expenses := []map[string]any{}
	for rows.Next() {
		var currency, status string
		var count, total int64
		if err = rows.Scan(&currency, &status, &count, &total); err != nil {
			catalogError(w, err)
			return
		}
		expenses = append(expenses, map[string]any{"currency": strings.TrimSpace(currency), "status": status, "count": count, "total_minor": total})
	}
	if err = rows.Err(); err != nil {
		catalogError(w, err)
		return
	}
	var closedShifts int64
	if err = tx.QueryRow(r.Context(), `SELECT count(*) FROM cash_shifts WHERE business_id=$1 AND branch_id=$2 AND status='closed' AND ($3='' OR closed_at::date >= $3::date) AND ($4='' OR closed_at::date <= $4::date) AND ($5::boolean OR user_id=$6::uuid)`, tc.BusinessID, tc.BranchID, from, to, hasPermission(tc.Role, "shifts.manage"), tc.UserID).Scan(&closedShifts); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"from": from, "to": to, "expenses_by_currency_status": expenses, "closed_shift_count": closedShifts, "currency_policy": "Totals are grouped by currency; no foreign-exchange conversion is applied."})
}
