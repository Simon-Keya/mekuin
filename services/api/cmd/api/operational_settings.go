package main

import (
	"github.com/jackc/pgx/v5"
	"net/http"
)

func (s *server) getOperationalSettings(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var threshold int64
	var low, out, expense, supplier, cash bool
	var updated any
	err := tx.QueryRow(r.Context(), `SELECT cash_variance_alert_threshold_minor,low_stock_alerts_enabled,stock_out_alerts_enabled,expense_approval_alerts_enabled,supplier_invoice_alerts_enabled,cash_variance_alerts_enabled,updated_at FROM branch_operational_settings WHERE business_id=$1 AND branch_id=$2`, tc.BusinessID, tc.BranchID).Scan(&threshold, &low, &out, &expense, &supplier, &cash, &updated)
	if err == pgx.ErrNoRows {
		threshold = 0
		low = true
		out = true
		expense = true
		supplier = true
		cash = true
	} else if err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"cash_variance_alert_threshold_minor": threshold, "low_stock_alerts_enabled": low, "stock_out_alerts_enabled": out, "expense_approval_alerts_enabled": expense, "supplier_invoice_alerts_enabled": supplier, "cash_variance_alerts_enabled": cash, "updated_at": updated})
}
func (s *server) updateOperationalSettings(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in struct {
		Threshold int64 `json:"cash_variance_alert_threshold_minor"`
		Low       bool  `json:"low_stock_alerts_enabled"`
		Out       bool  `json:"stock_out_alerts_enabled"`
		Expense   bool  `json:"expense_approval_alerts_enabled"`
		Supplier  bool  `json:"supplier_invoice_alerts_enabled"`
		Cash      bool  `json:"cash_variance_alerts_enabled"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Threshold < 0 || in.Threshold > 9_000_000_000_000 {
		jsonOut(w, 400, map[string]string{"error": "invalid_cash_variance_threshold"})
		return
	}
	_, err := tx.Exec(r.Context(), `INSERT INTO branch_operational_settings(business_id,branch_id,cash_variance_alert_threshold_minor,low_stock_alerts_enabled,stock_out_alerts_enabled,expense_approval_alerts_enabled,supplier_invoice_alerts_enabled,cash_variance_alerts_enabled,updated_by,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,now()) ON CONFLICT(business_id,branch_id) DO UPDATE SET cash_variance_alert_threshold_minor=EXCLUDED.cash_variance_alert_threshold_minor,low_stock_alerts_enabled=EXCLUDED.low_stock_alerts_enabled,stock_out_alerts_enabled=EXCLUDED.stock_out_alerts_enabled,expense_approval_alerts_enabled=EXCLUDED.expense_approval_alerts_enabled,supplier_invoice_alerts_enabled=EXCLUDED.supplier_invoice_alerts_enabled,cash_variance_alerts_enabled=EXCLUDED.cash_variance_alerts_enabled,updated_by=EXCLUDED.updated_by,updated_at=now()`, tc.BusinessID, tc.BranchID, in.Threshold, in.Low, in.Out, in.Expense, in.Supplier, in.Cash, tc.UserID)
	if err != nil {
		catalogError(w, err)
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,summary,metadata) VALUES($1,$2,'settings.operational.updated','branch_operational_settings','Branch operational alert settings updated',jsonb_build_object('branch_id',$3,'cash_variance_alert_threshold_minor',$4))`, tc.BusinessID, tc.UserID, tc.BranchID, in.Threshold)
	if err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"status": "updated"})
}
