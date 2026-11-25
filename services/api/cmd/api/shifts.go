package main

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type openCashShiftRequest struct {
	OpeningFloatMinor int64 `json:"opening_float_minor"`
}
type closeCashShiftRequest struct {
	CountedCashMinor int64 `json:"counted_cash_minor"`
}

func cashShiftExpectedCash(opening, sales, paidIn, paidOut int64) int64 {
	return opening + sales + paidIn - paidOut
}

type cashShiftMovementRequest struct {
	Direction   string `json:"direction"`
	AmountMinor int64  `json:"amount_minor"`
	Reason      string `json:"reason"`
}

func (s *server) openCashShift(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in openCashShiftRequest
	if !decode(w, r, &in) {
		return
	}
	if in.OpeningFloatMinor < 0 {
		jsonOut(w, 400, map[string]string{"error": "invalid_opening_float"})
		return
	}
	var id string
	err := tx.QueryRow(r.Context(), `INSERT INTO cash_shifts(business_id,branch_id,user_id,opening_float_minor) VALUES($1,$2,$3,$4) RETURNING id::text`, tc.BusinessID, tc.BranchID, tc.UserID, in.OpeningFloatMinor).Scan(&id)
	if err != nil {
		if pgErrCode(err) == "23505" {
			jsonOut(w, 409, map[string]string{"error": "cash_shift_already_open"})
			return
		}
		catalogError(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary,metadata) VALUES($1,$2,'shift.opened','cash_shift',$3,'Cash shift opened',jsonb_build_object('opening_float_minor',$4))`, tc.BusinessID, tc.UserID, id, in.OpeningFloatMinor); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 201, map[string]any{"shift_id": id, "status": "open", "opening_float_minor": in.OpeningFloatMinor})
}

func loadCashShiftTotals(r *http.Request, tx pgx.Tx, tc tenantContext, shiftID string) (int64, int64, int64, error) {
	var sales, paidIn, paidOut int64
	err := tx.QueryRow(r.Context(), `SELECT COALESCE(sum(p.amount_minor),0) FROM sale_payments p JOIN sales s ON s.business_id=p.business_id AND s.id=p.sale_id AND s.branch_id=p.branch_id WHERE p.business_id=$1 AND p.branch_id=$2 AND p.method='cash' AND p.status IN ('recorded','verified') AND s.status='completed' AND p.cash_shift_id=$3`, tc.BusinessID, tc.BranchID, shiftID).Scan(&sales)
	if err != nil {
		return 0, 0, 0, err
	}
	err = tx.QueryRow(r.Context(), `SELECT COALESCE(sum(amount_minor) FILTER(WHERE direction='paid_in'),0),COALESCE(sum(amount_minor) FILTER(WHERE direction='paid_out'),0) FROM cash_shift_movements WHERE business_id=$1 AND shift_id=$2`, tc.BusinessID, shiftID).Scan(&paidIn, &paidOut)
	return sales, paidIn, paidOut, err
}

func (s *server) getCurrentCashShift(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var v cashShiftView
	err := tx.QueryRow(r.Context(), `SELECT id::text,status,opening_float_minor,opened_at FROM cash_shifts WHERE business_id=$1 AND branch_id=$2 AND user_id=$3 AND status='open'`, tc.BusinessID, tc.BranchID, tc.UserID).Scan(&v.ID, &v.Status, &v.OpeningFloatMinor, &v.OpenedAt)
	if err == pgx.ErrNoRows {
		jsonOut(w, 200, map[string]any{"shift": nil})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	sales, paidIn, paidOut, err := loadCashShiftTotals(r, tx, tc, v.ID)
	if err != nil {
		catalogError(w, err)
		return
	}
	v.CashSalesMinor = sales
	v.PaidInMinor = paidIn
	v.PaidOutMinor = paidOut
	v.ExpectedCashMinor = cashShiftExpectedCash(v.OpeningFloatMinor, sales, paidIn, paidOut)
	jsonOut(w, 200, map[string]any{"shift": v})
}

type cashShiftView struct {
	ID                string     `json:"shift_id"`
	Status            string     `json:"status"`
	OpeningFloatMinor int64      `json:"opening_float_minor"`
	OpenedAt          time.Time  `json:"opened_at"`
	CashSalesMinor    int64      `json:"cash_sales_minor"`
	PaidInMinor       int64      `json:"paid_in_minor"`
	PaidOutMinor      int64      `json:"paid_out_minor"`
	ExpectedCashMinor int64      `json:"expected_cash_minor"`
	CountedCashMinor  *int64     `json:"counted_cash_minor,omitempty"`
	VarianceMinor     *int64     `json:"variance_minor,omitempty"`
	ClosedAt          *time.Time `json:"closed_at,omitempty"`
}

func (s *server) recordCashShiftMovement(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	shiftID := chi.URLParam(r, "shiftID")
	var in cashShiftMovementRequest
	if !decode(w, r, &in) {
		return
	}
	in.Direction = strings.TrimSpace(in.Direction)
	in.Reason = strings.TrimSpace(in.Reason)
	if !validUUID(shiftID) || (in.Direction != "paid_in" && in.Direction != "paid_out") || in.AmountMinor <= 0 || len(in.Reason) < 5 || len(in.Reason) > 300 {
		jsonOut(w, 400, map[string]string{"error": "invalid_cash_movement"})
		return
	}
	var owner string
	var opening int64
	err := tx.QueryRow(r.Context(), `SELECT user_id::text,opening_float_minor FROM cash_shifts WHERE business_id=$1 AND branch_id=$2 AND id=$3 AND status='open' FOR UPDATE`, tc.BusinessID, tc.BranchID, shiftID).Scan(&owner, &opening)
	if err == pgx.ErrNoRows {
		jsonOut(w, 404, map[string]string{"error": "open_shift_not_found"})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	if owner != tc.UserID && !hasPermission(tc.Role, "shifts.manage") {
		jsonOut(w, 403, map[string]string{"error": "shift_owner_required"})
		return
	}
	if in.Direction == "paid_out" {
		sales, paidIn, paidOut, totalsErr := loadCashShiftTotals(r, tx, tc, shiftID)
		if totalsErr != nil {
			catalogError(w, totalsErr)
			return
		}
		if in.AmountMinor > cashShiftExpectedCash(opening, sales, paidIn, paidOut) {
			jsonOut(w, 409, map[string]string{"error": "cash_drawer_insufficient_funds"})
			return
		}
	}
	var id string
	err = tx.QueryRow(r.Context(), `INSERT INTO cash_shift_movements(business_id,branch_id,shift_id,direction,amount_minor,reason,recorded_by) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id::text`, tc.BusinessID, tc.BranchID, shiftID, in.Direction, in.AmountMinor, in.Reason, tc.UserID).Scan(&id)
	if err != nil {
		catalogError(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary,metadata) VALUES($1,$2,'shift.cash_movement','cash_shift_movement',$3,'Cash paid-in/out recorded',jsonb_build_object('shift_id',$4,'direction',$5,'amount_minor',$6,'reason',$7))`, tc.BusinessID, tc.UserID, id, shiftID, in.Direction, in.AmountMinor, in.Reason); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 201, map[string]any{"movement_id": id, "direction": in.Direction, "amount_minor": in.AmountMinor})
}

func (s *server) closeCashShift(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	shiftID := chi.URLParam(r, "shiftID")
	var in closeCashShiftRequest
	if !decode(w, r, &in) {
		return
	}
	if !validUUID(shiftID) || in.CountedCashMinor < 0 {
		jsonOut(w, 400, map[string]string{"error": "invalid_shift_close"})
		return
	}
	var owner string
	var opening int64
	err := tx.QueryRow(r.Context(), `SELECT user_id::text,opening_float_minor FROM cash_shifts WHERE business_id=$1 AND branch_id=$2 AND id=$3 AND status='open' FOR UPDATE`, tc.BusinessID, tc.BranchID, shiftID).Scan(&owner, &opening)
	if err == pgx.ErrNoRows {
		jsonOut(w, 404, map[string]string{"error": "open_shift_not_found"})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	if owner != tc.UserID && !hasPermission(tc.Role, "shifts.manage") {
		jsonOut(w, 403, map[string]string{"error": "shift_owner_required"})
		return
	}
	sales, paidIn, paidOut, err := loadCashShiftTotals(r, tx, tc, shiftID)
	if err != nil {
		catalogError(w, err)
		return
	}
	expected := cashShiftExpectedCash(opening, sales, paidIn, paidOut)
	variance := in.CountedCashMinor - expected
	tag, err := tx.Exec(r.Context(), `UPDATE cash_shifts SET status='closed',expected_cash_minor=$4,counted_cash_minor=$5,variance_minor=$6,closed_at=now(),closed_by=$7 WHERE business_id=$1 AND branch_id=$2 AND id=$3 AND status='open'`, tc.BusinessID, tc.BranchID, shiftID, expected, in.CountedCashMinor, variance, tc.UserID)
	if err != nil {
		catalogError(w, err)
		return
	}
	if tag.RowsAffected() != 1 {
		jsonOut(w, 409, map[string]string{"error": "shift_close_conflict"})
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary,metadata) VALUES($1,$2,'shift.closed','cash_shift',$3,'Cash shift closed and reconciled',jsonb_build_object('opening_float_minor',$4,'cash_sales_minor',$5,'paid_in_minor',$6,'paid_out_minor',$7,'expected_cash_minor',$8,'counted_cash_minor',$9,'variance_minor',$10))`, tc.BusinessID, tc.UserID, shiftID, opening, sales, paidIn, paidOut, expected, in.CountedCashMinor, variance); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"shift_id": shiftID, "status": "closed", "opening_float_minor": opening, "cash_sales_minor": sales, "paid_in_minor": paidIn, "paid_out_minor": paidOut, "expected_cash_minor": expected, "counted_cash_minor": in.CountedCashMinor, "variance_minor": variance})
}

func (s *server) getCashShiftSummary(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	id := chi.URLParam(r, "shiftID")
	if !validUUID(id) {
		jsonOut(w, 400, map[string]string{"error": "invalid_shift_id"})
		return
	}
	var v cashShiftView
	var owner string
	err := tx.QueryRow(r.Context(), `SELECT id::text,status,user_id::text,opening_float_minor,expected_cash_minor,counted_cash_minor,variance_minor,opened_at,closed_at FROM cash_shifts WHERE business_id=$1 AND branch_id=$2 AND id=$3 AND status='closed'`, tc.BusinessID, tc.BranchID, id).Scan(&v.ID, &v.Status, &owner, &v.OpeningFloatMinor, &v.ExpectedCashMinor, &v.CountedCashMinor, &v.VarianceMinor, &v.OpenedAt, &v.ClosedAt)
	if err == pgx.ErrNoRows {
		jsonOut(w, 404, map[string]string{"error": "cash_shift_not_found"})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	if owner != tc.UserID && !hasPermission(tc.Role, "shifts.manage") {
		jsonOut(w, 403, map[string]string{"error": "shift_owner_required"})
		return
	}
	v.CashSalesMinor, v.PaidInMinor, v.PaidOutMinor, err = loadCashShiftTotals(r, tx, tc, id)
	if err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"shift": v})
}

func pgErrCode(err error) string {
	type sqlStater interface{ SQLState() string }
	if e, ok := err.(sqlStater); ok {
		return e.SQLState()
	}
	return ""
}
