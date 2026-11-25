package main

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type expenseCreateRequest struct {
	CategoryID  string `json:"category_id"`
	Description string `json:"description"`
	AmountMinor int64  `json:"amount_minor"`
	Currency    string `json:"currency"`
	ExpenseDate string `json:"expense_date"`
	Payee       string `json:"payee"`
	Reference   string `json:"reference"`
	Notes       string `json:"notes"`
}
type expenseReviewRequest struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}
type expensePayRequest struct {
	PaymentMethod    string `json:"payment_method"`
	PaymentReference string `json:"payment_reference"`
}

func validExpensePaymentMethod(method string) bool {
	switch method {
	case "cash", "bank_transfer", "mobile_money", "card", "other":
		return true
	default:
		return false
	}
}
func validateExpenseReview(decision, reason string) string {
	if decision != "approve" && decision != "reject" {
		return "invalid_review_decision"
	}
	if len(reason) > 2000 || (decision == "reject" && len(reason) < 5) {
		return "invalid_review_reason"
	}
	return ""
}

func (s *server) listExpenseCategories(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	rows, err := tx.Query(r.Context(), `SELECT id::text,name,is_active FROM expense_categories WHERE business_id=$1 ORDER BY name`, tc.BusinessID)
	if err != nil {
		catalogError(w, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name string
		var active bool
		if err = rows.Scan(&id, &name, &active); err != nil {
			catalogError(w, err)
			return
		}
		out = append(out, map[string]any{"id": id, "name": name, "is_active": active})
	}
	if err = rows.Err(); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"items": out})
}
func (s *server) createExpenseCategory(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if len(in.Name) < 2 || len(in.Name) > 80 {
		jsonOut(w, 400, map[string]string{"error": "invalid_category_name"})
		return
	}
	var id string
	err := tx.QueryRow(r.Context(), `INSERT INTO expense_categories(business_id,name) VALUES($1,$2) RETURNING id::text`, tc.BusinessID, in.Name).Scan(&id)
	if err != nil {
		catalogError(w, err)
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary) VALUES($1,$2,'expenses.category_created','expense_category',$3,'Expense category created')`, tc.BusinessID, tc.UserID, id)
	if err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 201, map[string]any{"id": id, "name": in.Name, "is_active": true})
}
func (s *server) listExpenses(w http.ResponseWriter, r *http.Request) {
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
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status != "" && status != "submitted" && status != "approved" && status != "rejected" && status != "paid" && status != "void" {
		jsonOut(w, 400, map[string]string{"error": "invalid_status"})
		return
	}
	rows, err := tx.Query(r.Context(), `SELECT e.id::text,e.category_id::text,c.name,e.description,e.amount_minor,e.currency,e.expense_date::text,e.payee,e.reference,e.status,e.created_by::text,e.created_at,e.reviewed_by::text,e.reviewed_at,e.rejection_reason,e.paid_at,p.payment_method,p.payment_reference FROM expenses e JOIN expense_categories c ON c.business_id=e.business_id AND c.id=e.category_id LEFT JOIN expense_payments p ON p.business_id=e.business_id AND p.expense_id=e.id WHERE e.business_id=$1 AND e.branch_id=$2 AND ($3='' OR e.status=$3) AND ($4='' OR e.description ILIKE '%'||$4||'%' OR c.name ILIKE '%'||$4||'%' OR COALESCE(e.payee,'') ILIKE '%'||$4||'%') ORDER BY e.expense_date DESC,e.created_at DESC LIMIT $5 OFFSET $6`, tc.BusinessID, tc.BranchID, status, q, limit, offset)
	if err != nil {
		catalogError(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, catID, cat, desc, currency, date, status, createdBy string
		var amount int64
		var payee, ref, reviewedBy, reject, paymentMethod, paymentRef *string
		var created time.Time
		var reviewedAt, paidAt *time.Time
		if err = rows.Scan(&id, &catID, &cat, &desc, &amount, &currency, &date, &payee, &ref, &status, &createdBy, &created, &reviewedBy, &reviewedAt, &reject, &paidAt, &paymentMethod, &paymentRef); err != nil {
			catalogError(w, err)
			return
		}
		items = append(items, map[string]any{"id": id, "category_id": catID, "category_name": cat, "description": desc, "amount_minor": amount, "currency": strings.TrimSpace(currency), "expense_date": date, "payee": payee, "reference": ref, "status": status, "created_by": createdBy, "created_at": created, "reviewed_by": reviewedBy, "reviewed_at": reviewedAt, "rejection_reason": reject, "paid_at": paidAt, "payment_method": paymentMethod, "payment_reference": paymentRef})
	}
	if err = rows.Err(); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"items": items})
}
func (s *server) listExpenseReviewEvents(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	id := chi.URLParam(r, "expenseID")
	rows, err := tx.Query(r.Context(), `SELECT id::text,event_no,decision,reason,actor_user_id::text,created_at FROM expense_review_events WHERE business_id=$1 AND branch_id=$2 AND expense_id=$3 ORDER BY event_no`, tc.BusinessID, tc.BranchID, id)
	if err != nil {
		catalogError(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var eventID, decision, actor string
		var no int
		var reason *string
		var created time.Time
		if err = rows.Scan(&eventID, &no, &decision, &reason, &actor, &created); err != nil {
			catalogError(w, err)
			return
		}
		items = append(items, map[string]any{"id": eventID, "event_no": no, "decision": decision, "reason": reason, "actor_user_id": actor, "created_at": created})
	}
	if err = rows.Err(); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"items": items})
}

func (s *server) createExpense(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in expenseCreateRequest
	if !decode(w, r, &in) {
		return
	}
	in.Description = strings.TrimSpace(in.Description)
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	in.Payee = strings.TrimSpace(in.Payee)
	in.Reference = strings.TrimSpace(in.Reference)
	in.Notes = strings.TrimSpace(in.Notes)
	if len(in.Description) < 3 || len(in.Description) > 500 || in.AmountMinor <= 0 || !validCurrencyCode(in.Currency) || !validISODate(in.ExpenseDate, false) || len(in.Payee) > 160 || len(in.Reference) > 160 || len(in.Notes) > 2000 {
		jsonOut(w, 400, map[string]string{"error": "invalid_expense"})
		return
	}
	var id string
	err := tx.QueryRow(r.Context(), `INSERT INTO expenses(business_id,branch_id,category_id,description,amount_minor,currency,expense_date,payee,reference,notes,created_by) SELECT $1,$2,c.id,$4,$5,$6,$7::date,NULLIF($8,''),NULLIF($9,''),NULLIF($10,''),$11 FROM expense_categories c WHERE c.business_id=$1 AND c.id=$3 AND c.is_active RETURNING id::text`, tc.BusinessID, tc.BranchID, in.CategoryID, in.Description, in.AmountMinor, in.Currency, in.ExpenseDate, in.Payee, in.Reference, in.Notes, tc.UserID).Scan(&id)
	if err == pgx.ErrNoRows {
		jsonOut(w, 422, map[string]string{"error": "active_expense_category_required"})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary,metadata) VALUES($1,$2,'expenses.submitted','expense',$3,'Expense submitted for approval',jsonb_build_object('amount_minor',$4,'currency',$5,'category_id',$6))`, tc.BusinessID, tc.UserID, id, in.AmountMinor, in.Currency, in.CategoryID); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 201, map[string]any{"id": id, "status": "submitted"})
}
func (s *server) reviewExpense(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	id := chi.URLParam(r, "expenseID")
	var in expenseReviewRequest
	if !decode(w, r, &in) {
		return
	}
	in.Decision = strings.ToLower(strings.TrimSpace(in.Decision))
	in.Reason = strings.TrimSpace(in.Reason)
	if problem := validateExpenseReview(in.Decision, in.Reason); problem != "" {
		jsonOut(w, 400, map[string]string{"error": problem})
		return
	}
	var status, creator string
	err := tx.QueryRow(r.Context(), `SELECT status,created_by::text FROM expenses WHERE business_id=$1 AND branch_id=$2 AND id=$3 FOR UPDATE`, tc.BusinessID, tc.BranchID, id).Scan(&status, &creator)
	if err == pgx.ErrNoRows {
		jsonOut(w, 404, map[string]string{"error": "expense_not_found"})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	if status != "submitted" {
		jsonOut(w, 409, map[string]string{"error": "expense_not_pending_review"})
		return
	}
	if creator == tc.UserID {
		jsonOut(w, 403, map[string]string{"error": "expense_creator_cannot_review"})
		return
	}
	next, event := "approved", "approved"
	if in.Decision == "reject" {
		next, event = "rejected", "rejected"
	}
	_, err = tx.Exec(r.Context(), `UPDATE expenses SET status=$4,reviewed_by=$5,reviewed_at=now(),rejection_reason=NULLIF($6,'') WHERE business_id=$1 AND branch_id=$2 AND id=$3`, tc.BusinessID, tc.BranchID, id, next, tc.UserID, in.Reason)
	if err != nil {
		catalogError(w, err)
		return
	}
	var eventNo int
	err = tx.QueryRow(r.Context(), `SELECT COALESCE(max(event_no),0)+1 FROM expense_review_events WHERE business_id=$1 AND expense_id=$2`, tc.BusinessID, id).Scan(&eventNo)
	if err != nil {
		catalogError(w, err)
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO expense_review_events(business_id,branch_id,expense_id,event_no,decision,reason,actor_user_id) VALUES($1,$2,$3,$4,$5,NULLIF($6,''),$7)`, tc.BusinessID, tc.BranchID, id, eventNo, event, in.Reason, tc.UserID)
	if err != nil {
		catalogError(w, err)
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary,metadata) VALUES($1,$2,$3,'expense',$4,'Expense approval decision recorded',jsonb_build_object('decision',$5,'reason',$6))`, tc.BusinessID, tc.UserID, "expenses."+event, id, event, in.Reason)
	if err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]string{"id": id, "status": next})
}
func (s *server) payExpense(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	id := chi.URLParam(r, "expenseID")
	var in expensePayRequest
	if !decode(w, r, &in) {
		return
	}
	in.PaymentMethod = strings.ToLower(strings.TrimSpace(in.PaymentMethod))
	in.PaymentReference = strings.TrimSpace(in.PaymentReference)
	if !validExpensePaymentMethod(in.PaymentMethod) || len(in.PaymentReference) > 160 {
		jsonOut(w, 400, map[string]string{"error": "invalid_payment_method_or_reference"})
		return
	}
	var amount int64
	var status string
	err := tx.QueryRow(r.Context(), `SELECT amount_minor,status FROM expenses WHERE business_id=$1 AND branch_id=$2 AND id=$3 FOR UPDATE`, tc.BusinessID, tc.BranchID, id).Scan(&amount, &status)
	if err == pgx.ErrNoRows {
		jsonOut(w, 404, map[string]string{"error": "expense_not_found"})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	if status != "approved" {
		jsonOut(w, 409, map[string]string{"error": "approved_expense_required"})
		return
	}
	var shiftID any
	if in.PaymentMethod == "cash" {
		var sid string
		err = tx.QueryRow(r.Context(), `SELECT id::text FROM cash_shifts WHERE business_id=$1 AND branch_id=$2 AND user_id=$3 AND status='open' FOR UPDATE`, tc.BusinessID, tc.BranchID, tc.UserID).Scan(&sid)
		if err == pgx.ErrNoRows {
			jsonOut(w, 409, map[string]string{"error": "cash_shift_required"})
			return
		}
		if err != nil {
			catalogError(w, err)
			return
		}
		var opening, sales, paidIn, paidOut int64
		err = tx.QueryRow(r.Context(), `SELECT opening_float_minor FROM cash_shifts WHERE business_id=$1 AND id=$2`, tc.BusinessID, sid).Scan(&opening)
		if err != nil {
			catalogError(w, err)
			return
		}
		sales, paidIn, paidOut, err = loadCashShiftTotals(r, tx, tc, sid)
		if err != nil {
			catalogError(w, err)
			return
		}
		if cashShiftExpectedCash(opening, sales, paidIn, paidOut) < amount {
			jsonOut(w, 409, map[string]string{"error": "insufficient_shift_cash"})
			return
		}
		shiftID = sid
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO expense_payments(business_id,branch_id,expense_id,payment_method,amount_minor,payment_reference,cash_shift_id,recorded_by) VALUES($1,$2,$3,$4,$5,NULLIF($6,''),$7,$8)`, tc.BusinessID, tc.BranchID, id, in.PaymentMethod, amount, in.PaymentReference, shiftID, tc.UserID)
	if err != nil {
		catalogError(w, err)
		return
	}
	if in.PaymentMethod == "cash" {
		_, err = tx.Exec(r.Context(), `INSERT INTO cash_shift_movements(business_id,branch_id,shift_id,direction,amount_minor,reason,recorded_by) VALUES($1,$2,$3,'paid_out',$4,$5,$6)`, tc.BusinessID, tc.BranchID, shiftID, amount, "Expense payment: "+id, tc.UserID)
		if err != nil {
			catalogError(w, err)
			return
		}
	}
	_, err = tx.Exec(r.Context(), `UPDATE expenses SET status='paid',paid_at=now() WHERE business_id=$1 AND branch_id=$2 AND id=$3`, tc.BusinessID, tc.BranchID, id)
	if err != nil {
		catalogError(w, err)
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary,metadata) VALUES($1,$2,'expenses.paid','expense',$3,'Approved expense paid',jsonb_build_object('amount_minor',$4,'payment_method',$5,'cash_shift_id',$6))`, tc.BusinessID, tc.UserID, id, amount, in.PaymentMethod, shiftID)
	if err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"id": id, "status": "paid", "amount_minor": amount, "payment_method": in.PaymentMethod})
}
func (s *server) expenseSummaryReport(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	if from != "" && !validISODate(from, false) || to != "" && !validISODate(to, false) || from != "" && to != "" && from > to {
		jsonOut(w, 400, map[string]string{"error": "invalid_date_range"})
		return
	}
	rows, err := tx.Query(r.Context(), `SELECT c.name,e.currency,e.status,count(*)::bigint,sum(e.amount_minor)::bigint FROM expenses e JOIN expense_categories c ON c.business_id=e.business_id AND c.id=e.category_id WHERE e.business_id=$1 AND e.branch_id=$2 AND ($3='' OR e.expense_date >= $3::date) AND ($4='' OR e.expense_date <= $4::date) GROUP BY c.name,e.currency,e.status ORDER BY c.name,e.currency,e.status`, tc.BusinessID, tc.BranchID, from, to)
	if err != nil {
		catalogError(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var category, currency, status string
		var count, total int64
		if err = rows.Scan(&category, &currency, &status, &count, &total); err != nil {
			catalogError(w, err)
			return
		}
		items = append(items, map[string]any{"category": category, "currency": strings.TrimSpace(currency), "status": status, "count": count, "total_minor": total})
	}
	if err = rows.Err(); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"items": items, "from": from, "to": to})
}
