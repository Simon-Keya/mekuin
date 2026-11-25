package main

import (
	"database/sql"
	"math/big"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type supplierInvoiceLineInput struct {
	PurchaseOrderLineID string `json:"purchase_order_line_id"`
	ProductID           string `json:"product_id"`
	Description         string `json:"description"`
	Quantity            string `json:"quantity"`
	UnitCostMinor       int64  `json:"unit_cost_minor"`
	LineTotalMinor      int64  `json:"line_total_minor"`
}
type supplierInvoiceInput struct {
	SupplierID        string                     `json:"supplier_id"`
	PurchaseOrderID   string                     `json:"purchase_order_id"`
	InvoiceNumber     string                     `json:"invoice_number"`
	InvoiceDate       string                     `json:"invoice_date"`
	DueDate           string                     `json:"due_date"`
	Currency          string                     `json:"currency"`
	TotalMinor        int64                      `json:"total_minor"`
	SupplierReference string                     `json:"supplier_reference"`
	Notes             string                     `json:"notes"`
	Lines             []supplierInvoiceLineInput `json:"lines"`
}
type supplierPaymentAllocationInput struct {
	InvoiceID   string `json:"invoice_id"`
	AmountMinor int64  `json:"amount_minor"`
}
type supplierPaymentInput struct {
	SupplierID        string                           `json:"supplier_id"`
	PaymentReference  string                           `json:"payment_reference"`
	PaymentDate       string                           `json:"payment_date"`
	AmountMinor       int64                            `json:"amount_minor"`
	Currency          string                           `json:"currency"`
	PaymentMethod     string                           `json:"payment_method"`
	ExternalReference string                           `json:"external_reference"`
	Notes             string                           `json:"notes"`
	Allocations       []supplierPaymentAllocationInput `json:"allocations"`
}
type supplierInvoiceReviewInput struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

func invoiceReviewAllowsPayment(reviewStatus string) bool {
	return reviewStatus == "approved" || reviewStatus == "approved_with_exceptions"
}

func validateInvoiceReviewDecision(decision, reason string, hasExceptions bool) string {
	reason = strings.TrimSpace(reason)
	switch decision {
	case "approve":
		if hasExceptions {
			return "invoice_has_matching_exceptions"
		}
		if reason != "" && len(reason) > 2000 {
			return "invalid_review_reason"
		}
	case "approve_with_exceptions":
		if !hasExceptions {
			return "invoice_has_no_matching_exceptions"
		}
		if !requiredText(reason, 2000) {
			return "exception_resolution_reason_required"
		}
	case "reject":
		if !requiredText(reason, 2000) {
			return "rejection_reason_required"
		}
	default:
		return "invalid_review_decision"
	}
	return ""
}

func validCurrencyCode(value string) bool {
	if len(value) != 3 {
		return false
	}
	for _, c := range value {
		if c < 'A' || c > 'Z' {
			return false
		}
	}
	return true
}

func validISODate(value string, optional bool) bool {
	if optional && value == "" {
		return true
	}
	if len(value) != 10 {
		return false
	}
	_, err := time.Parse("2006-01-02", value)
	return err == nil
}

func sumMinor(values []int64) (int64, bool) {
	total := new(big.Int)
	for _, value := range values {
		total.Add(total, big.NewInt(value))
	}
	if !total.IsInt64() {
		return 0, false
	}
	return total.Int64(), true
}

func (s *server) listSupplierInvoices(w http.ResponseWriter, r *http.Request) {
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
	rows, err := tx.Query(r.Context(), `SELECT i.id,i.supplier_id,s.name,i.invoice_number,i.invoice_date,i.due_date,i.currency,i.total_minor,CASE WHEN i.status='void' THEN 'void' WHEN COALESCE(a.allocated_minor,0)>=i.total_minor THEN 'paid' WHEN COALESCE(a.allocated_minor,0)>0 THEN 'partially_paid' ELSE i.status END,COALESCE(a.allocated_minor,0)::bigint,GREATEST(i.total_minor-COALESCE(a.allocated_minor,0),0)::bigint AS outstanding_minor,COALESCE(review.event_type,'not_submitted'),i.created_at FROM supplier_invoices i JOIN suppliers s ON s.business_id=i.business_id AND s.id=i.supplier_id LEFT JOIN (SELECT business_id,invoice_id,sum(amount_minor)::bigint allocated_minor FROM supplier_payment_allocations GROUP BY business_id,invoice_id) a ON a.business_id=i.business_id AND a.invoice_id=i.id LEFT JOIN LATERAL (SELECT event_type FROM supplier_invoice_review_events re WHERE re.business_id=i.business_id AND re.invoice_id=i.id ORDER BY re.event_no DESC LIMIT 1) review ON TRUE WHERE i.business_id=$1 AND i.branch_id=$2 AND ($3='' OR i.invoice_number ILIKE '%'||$3||'%' OR s.name ILIKE '%'||$3||'%') ORDER BY i.invoice_date DESC,i.created_at DESC LIMIT $4 OFFSET $5`, tc.BusinessID, tc.BranchID, q, limit, offset)
	if err != nil {
		catalogError(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, supplier, name, num, date, currency, status, reviewStatus string
		var due *string
		var total, paid, outstanding int64
		var created any
		if err = rows.Scan(&id, &supplier, &name, &num, &date, &due, &currency, &total, &status, &paid, &outstanding, &reviewStatus, &created); err != nil {
			catalogError(w, err)
			return
		}
		items = append(items, map[string]any{"id": id, "supplier_id": supplier, "supplier_name": name, "invoice_number": num, "invoice_date": date, "due_date": due, "currency": strings.TrimSpace(currency), "total_minor": total, "allocated_minor": paid, "outstanding_minor": outstanding, "status": status, "review_status": reviewStatus, "created_at": created})
	}
	if rows.Err() != nil {
		catalogError(w, rows.Err())
		return
	}
	jsonOut(w, 200, map[string]any{"items": items, "limit": limit, "offset": offset})
}

func (s *server) createSupplierInvoice(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in supplierInvoiceInput
	if !decode(w, r, &in) {
		return
	}
	in.SupplierID = strings.ToLower(strings.TrimSpace(in.SupplierID))
	in.PurchaseOrderID = strings.ToLower(strings.TrimSpace(in.PurchaseOrderID))
	in.InvoiceNumber = strings.TrimSpace(in.InvoiceNumber)
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	in.SupplierReference = strings.TrimSpace(in.SupplierReference)
	in.Notes = strings.TrimSpace(in.Notes)
	if !validUUID(in.SupplierID) || !requiredText(in.InvoiceNumber, 100) || !validISODate(in.InvoiceDate, false) || !validISODate(in.DueDate, true) || !validCurrencyCode(in.Currency) || in.TotalMinor < 0 || len(in.Lines) == 0 || len(in.Lines) > 100 || len(in.Notes) > 2000 || len(in.SupplierReference) > 160 {
		jsonOut(w, 400, map[string]string{"error": "invalid_supplier_invoice"})
		return
	}
	if in.PurchaseOrderID != "" && !validUUID(in.PurchaseOrderID) {
		jsonOut(w, 400, map[string]string{"error": "invalid_purchase_order_id"})
		return
	}
	var supplier string
	err := tx.QueryRow(r.Context(), `SELECT id FROM suppliers WHERE business_id=$1 AND id=$2`, tc.BusinessID, in.SupplierID).Scan(&supplier)
	if err == pgx.ErrNoRows {
		jsonOut(w, 404, map[string]string{"error": "supplier_not_found"})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	if in.PurchaseOrderID != "" {
		var poSupplier string
		err = tx.QueryRow(r.Context(), `SELECT supplier_id FROM purchase_orders WHERE business_id=$1 AND branch_id=$2 AND id=$3`, tc.BusinessID, tc.BranchID, in.PurchaseOrderID).Scan(&poSupplier)
		if err == pgx.ErrNoRows {
			jsonOut(w, 404, map[string]string{"error": "purchase_order_not_found"})
			return
		}
		if err != nil {
			catalogError(w, err)
			return
		}
		if poSupplier != in.SupplierID {
			jsonOut(w, 422, map[string]string{"error": "purchase_order_supplier_mismatch"})
			return
		}
	}
	lineTotals := make([]int64, 0, len(in.Lines))
	seenPurchaseOrderLines := map[string]bool{}
	for i := range in.Lines {
		l := &in.Lines[i]
		l.ProductID = strings.ToLower(strings.TrimSpace(l.ProductID))
		l.PurchaseOrderLineID = strings.ToLower(strings.TrimSpace(l.PurchaseOrderLineID))
		l.Description = strings.TrimSpace(l.Description)
		l.Quantity = strings.TrimSpace(l.Quantity)
		if !validUUID(l.ProductID) || !requiredText(l.Description, 240) || !inventoryQuantityPattern.MatchString(l.Quantity) || strings.Trim(l.Quantity, "0.") == "" || l.UnitCostMinor < 0 || l.LineTotalMinor < 0 {
			jsonOut(w, 400, map[string]string{"error": "invalid_supplier_invoice_line"})
			return
		}
		if l.PurchaseOrderLineID != "" && !validUUID(l.PurchaseOrderLineID) {
			jsonOut(w, 400, map[string]string{"error": "invalid_purchase_order_line_id"})
			return
		}
		var matches bool
		if l.PurchaseOrderLineID != "" {
			if seenPurchaseOrderLines[l.PurchaseOrderLineID] {
				jsonOut(w, 400, map[string]string{"error": "duplicate_purchase_order_line"})
				return
			}
			seenPurchaseOrderLines[l.PurchaseOrderLineID] = true
			if in.PurchaseOrderID == "" {
				jsonOut(w, 422, map[string]string{"error": "purchase_order_required_for_linked_line"})
				return
			}
			err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM purchase_order_lines pol WHERE pol.business_id=$1 AND pol.purchase_order_id=$2 AND pol.id=$3 AND pol.product_id=$4)`, tc.BusinessID, in.PurchaseOrderID, l.PurchaseOrderLineID, l.ProductID).Scan(&matches)
		} else {
			err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM products WHERE business_id=$1 AND id=$2)`, tc.BusinessID, l.ProductID).Scan(&matches)
		}
		if err != nil {
			catalogError(w, err)
			return
		}
		if !matches {
			jsonOut(w, 422, map[string]string{"error": "invoice_line_product_or_order_mismatch"})
			return
		}
		var calculated int64
		err = tx.QueryRow(r.Context(), `SELECT round($1::numeric*$2::numeric)::bigint`, l.Quantity, l.UnitCostMinor).Scan(&calculated)
		if err != nil {
			catalogError(w, err)
			return
		}
		if calculated != l.LineTotalMinor {
			jsonOut(w, 422, map[string]string{"error": "invoice_line_total_mismatch"})
			return
		}
		lineTotals = append(lineTotals, l.LineTotalMinor)
	}
	total, ok := sumMinor(lineTotals)
	if !ok || total != in.TotalMinor {
		jsonOut(w, 422, map[string]string{"error": "invoice_header_total_mismatch"})
		return
	}
	var id string
	err = tx.QueryRow(r.Context(), `INSERT INTO supplier_invoices(business_id,branch_id,supplier_id,purchase_order_id,invoice_number,invoice_date,due_date,currency,total_minor,status,supplier_reference,notes,created_by) VALUES($1,$2,$3,NULLIF($4,'')::uuid,$5,$6::date,NULLIF($7,'')::date,$8,$9,'recorded',NULLIF($10,''),NULLIF($11,''),$12) RETURNING id`, tc.BusinessID, tc.BranchID, in.SupplierID, in.PurchaseOrderID, in.InvoiceNumber, in.InvoiceDate, in.DueDate, in.Currency, in.TotalMinor, in.SupplierReference, in.Notes, tc.UserID).Scan(&id)
	if err != nil {
		catalogError(w, err)
		return
	}
	for _, l := range in.Lines {
		_, err = tx.Exec(r.Context(), `INSERT INTO supplier_invoice_lines(business_id,invoice_id,purchase_order_line_id,product_id,description,quantity,unit_cost_minor,line_total_minor) VALUES($1,$2,NULLIF($3,'')::uuid,$4,$5,$6::numeric,$7,$8)`, tc.BusinessID, id, l.PurchaseOrderLineID, l.ProductID, l.Description, l.Quantity, l.UnitCostMinor, l.LineTotalMinor)
		if err != nil {
			catalogError(w, err)
			return
		}
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary,metadata) VALUES($1,$2,'payables.invoice_recorded','supplier_invoice',$3,'Supplier invoice recorded',jsonb_build_object('supplier_id',$4,'total_minor',$5,'currency',$6,'line_count',$7))`, tc.BusinessID, tc.UserID, id, in.SupplierID, in.TotalMinor, in.Currency, len(in.Lines))
	if err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 201, map[string]any{"id": id, "status": "recorded", "total_minor": in.TotalMinor, "currency": in.Currency})
}

func (s *server) getSupplierInvoice(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	id := chi.URLParam(r, "invoiceID")
	var supplier, name, num, date, currency, status string
	var due, reference, notes *string
	var total int64
	var created any
	err := tx.QueryRow(r.Context(), `SELECT i.supplier_id,s.name,i.invoice_number,i.invoice_date,i.due_date,i.currency,i.total_minor,i.status,i.supplier_reference,i.notes,i.created_at FROM supplier_invoices i JOIN suppliers s ON s.business_id=i.business_id AND s.id=i.supplier_id WHERE i.business_id=$1 AND i.branch_id=$2 AND i.id=$3`, tc.BusinessID, tc.BranchID, id).Scan(&supplier, &name, &num, &date, &due, &currency, &total, &status, &reference, &notes, &created)
	if err == pgx.ErrNoRows {
		jsonOut(w, 404, map[string]string{"error": "supplier_invoice_not_found"})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	var paid int64
	err = tx.QueryRow(r.Context(), `SELECT COALESCE(sum(amount_minor),0)::bigint FROM supplier_payment_allocations WHERE business_id=$1 AND invoice_id=$2`, tc.BusinessID, id).Scan(&paid)
	if err != nil {
		catalogError(w, err)
		return
	}
	rows, err := tx.Query(r.Context(), `SELECT l.id,l.purchase_order_line_id,l.product_id,p.name,l.description,l.quantity::text,l.unit_cost_minor,l.line_total_minor FROM supplier_invoice_lines l JOIN products p ON p.business_id=l.business_id AND p.id=l.product_id WHERE l.business_id=$1 AND l.invoice_id=$2 ORDER BY l.created_at,l.id`, tc.BusinessID, id)
	if err != nil {
		catalogError(w, err)
		return
	}
	defer rows.Close()
	lines := []map[string]any{}
	for rows.Next() {
		var lid, pid, pname, desc, qty string
		var poLine *string
		var unit, line int64
		if err = rows.Scan(&lid, &poLine, &pid, &pname, &desc, &qty, &unit, &line); err != nil {
			catalogError(w, err)
			return
		}
		lines = append(lines, map[string]any{"id": lid, "purchase_order_line_id": poLine, "product_id": pid, "product_name": pname, "description": desc, "quantity": qty, "unit_cost_minor": unit, "line_total_minor": line})
	}
	if rows.Err() != nil {
		catalogError(w, rows.Err())
		return
	}
	var reviewStatus string
	err = tx.QueryRow(r.Context(), `SELECT COALESCE((SELECT event_type FROM supplier_invoice_review_events WHERE business_id=$1 AND invoice_id=$2 ORDER BY event_no DESC LIMIT 1),'not_submitted')`, tc.BusinessID, id).Scan(&reviewStatus)
	if err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"id": id, "supplier_id": supplier, "supplier_name": name, "invoice_number": num, "invoice_date": date, "due_date": due, "currency": strings.TrimSpace(currency), "total_minor": total, "allocated_minor": paid, "outstanding_minor": maxInt64(total-paid, 0), "status": effectiveInvoiceStatus(status, paid, total), "review_status": reviewStatus, "supplier_reference": reference, "notes": notes, "created_at": created, "lines": lines})
}

func (s *server) createSupplierPayment(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in supplierPaymentInput
	if !decode(w, r, &in) {
		return
	}
	in.SupplierID = strings.ToLower(strings.TrimSpace(in.SupplierID))
	in.PaymentReference = strings.TrimSpace(in.PaymentReference)
	in.PaymentDate = strings.TrimSpace(in.PaymentDate)
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	in.PaymentMethod = strings.TrimSpace(in.PaymentMethod)
	in.ExternalReference = strings.TrimSpace(in.ExternalReference)
	in.Notes = strings.TrimSpace(in.Notes)
	methods := map[string]bool{"cash": true, "bank_transfer": true, "card": true, "mobile_money": true, "other": true}
	if !validUUID(in.SupplierID) || !requiredText(in.PaymentReference, 120) || !validISODate(in.PaymentDate, false) || in.AmountMinor <= 0 || !validCurrencyCode(in.Currency) || !methods[in.PaymentMethod] || len(in.ExternalReference) > 160 || len(in.Notes) > 2000 || len(in.Allocations) > 100 {
		jsonOut(w, 400, map[string]string{"error": "invalid_supplier_payment"})
		return
	}
	if len(in.Allocations) == 0 {
		jsonOut(w, 422, map[string]string{"error": "payment_requires_invoice_allocations"})
		return
	}
	seen := map[string]bool{}
	ids := make([]string, 0, len(in.Allocations))
	amounts := make([]int64, 0, len(in.Allocations))
	for i := range in.Allocations {
		a := &in.Allocations[i]
		a.InvoiceID = strings.ToLower(strings.TrimSpace(a.InvoiceID))
		if !validUUID(a.InvoiceID) || seen[a.InvoiceID] || a.AmountMinor <= 0 {
			jsonOut(w, 400, map[string]string{"error": "invalid_payment_allocation"})
			return
		}
		seen[a.InvoiceID] = true
		ids = append(ids, a.InvoiceID)
		amounts = append(amounts, a.AmountMinor)
	}
	allocated, ok := sumMinor(amounts)
	if !ok || allocated > in.AmountMinor {
		jsonOut(w, 422, map[string]string{"error": "allocations_exceed_payment_amount"})
		return
	}
	sort.Strings(ids)
	// Lock invoice rows in a deterministic order so concurrent payments cannot
	// both consume the same remaining balance.
	for _, id := range ids {
		var supplier, currency, status, reviewStatus string
		var total int64
		err := tx.QueryRow(r.Context(), `SELECT i.supplier_id,i.currency,i.total_minor,i.status,COALESCE((SELECT re.event_type FROM supplier_invoice_review_events re WHERE re.business_id=i.business_id AND re.invoice_id=i.id ORDER BY re.event_no DESC LIMIT 1),'not_submitted') FROM supplier_invoices i WHERE i.business_id=$1 AND i.branch_id=$2 AND i.id=$3 FOR UPDATE`, tc.BusinessID, tc.BranchID, id).Scan(&supplier, &currency, &total, &status, &reviewStatus)
		if err == pgx.ErrNoRows {
			jsonOut(w, 404, map[string]string{"error": "supplier_invoice_not_found"})
			return
		}
		if err != nil {
			catalogError(w, err)
			return
		}
		if supplier != in.SupplierID || strings.TrimSpace(currency) != in.Currency || status == "void" {
			jsonOut(w, 422, map[string]string{"error": "payment_invoice_supplier_currency_or_status_mismatch"})
			return
		}
		if !invoiceReviewAllowsPayment(reviewStatus) {
			jsonOut(w, 409, map[string]string{"error": "supplier_invoice_not_approved_for_payment", "review_status": reviewStatus})
			return
		}
		var requested int64
		for _, a := range in.Allocations {
			if a.InvoiceID == id {
				requested = a.AmountMinor
				break
			}
		}
		var paid int64
		err = tx.QueryRow(r.Context(), `SELECT COALESCE(sum(amount_minor),0)::bigint FROM supplier_payment_allocations WHERE business_id=$1 AND invoice_id=$2`, tc.BusinessID, id).Scan(&paid)
		if err != nil {
			catalogError(w, err)
			return
		}
		if paid > total || requested > total-paid {
			jsonOut(w, 409, map[string]string{"error": "invoice_outstanding_balance_exceeded"})
			return
		}
	}
	var paymentID string
	err := tx.QueryRow(r.Context(), `INSERT INTO supplier_payments(business_id,branch_id,supplier_id,payment_reference,payment_date,amount_minor,currency,payment_method,external_reference,notes,recorded_by) VALUES($1,$2,$3,$4,$5::date,$6,$7,$8,NULLIF($9,''),NULLIF($10,''),$11) RETURNING id`, tc.BusinessID, tc.BranchID, in.SupplierID, in.PaymentReference, in.PaymentDate, in.AmountMinor, in.Currency, in.PaymentMethod, in.ExternalReference, in.Notes, tc.UserID).Scan(&paymentID)
	if err != nil {
		catalogError(w, err)
		return
	}
	for _, a := range in.Allocations {
		_, err = tx.Exec(r.Context(), `INSERT INTO supplier_payment_allocations(business_id,payment_id,invoice_id,amount_minor) VALUES($1,$2,$3,$4)`, tc.BusinessID, paymentID, a.InvoiceID, a.AmountMinor)
		if err != nil {
			catalogError(w, err)
			return
		}
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary,metadata) VALUES($1,$2,'payables.payment_recorded','supplier_payment',$3,'Supplier payment recorded and allocated',jsonb_build_object('supplier_id',$4,'amount_minor',$5,'allocated_minor',$6,'currency',$7,'allocation_count',$8))`, tc.BusinessID, tc.UserID, paymentID, in.SupplierID, in.AmountMinor, allocated, in.Currency, len(in.Allocations))
	if err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 201, map[string]any{"id": paymentID, "amount_minor": in.AmountMinor, "allocated_minor": allocated, "unallocated_minor": in.AmountMinor - allocated, "currency": in.Currency})
}

func (s *server) listSupplierPayments(w http.ResponseWriter, r *http.Request) {
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
	rows, err := tx.Query(r.Context(), `SELECT p.id,p.supplier_id,s.name,p.payment_reference,p.payment_date,p.amount_minor,p.currency,p.payment_method,p.external_reference,COALESCE(a.allocated_minor,0)::bigint,p.created_at FROM supplier_payments p JOIN suppliers s ON s.business_id=p.business_id AND s.id=p.supplier_id LEFT JOIN (SELECT business_id,payment_id,sum(amount_minor)::bigint allocated_minor FROM supplier_payment_allocations GROUP BY business_id,payment_id) a ON a.business_id=p.business_id AND a.payment_id=p.id WHERE p.business_id=$1 AND p.branch_id=$2 AND ($3='' OR p.payment_reference ILIKE '%'||$3||'%' OR s.name ILIKE '%'||$3||'%') ORDER BY p.payment_date DESC,p.created_at DESC LIMIT $4 OFFSET $5`, tc.BusinessID, tc.BranchID, q, limit, offset)
	if err != nil {
		catalogError(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, supplier, name, ref, date, currency, method string
		var external *string
		var amount, allocated int64
		var created any
		if err = rows.Scan(&id, &supplier, &name, &ref, &date, &amount, &currency, &method, &external, &allocated, &created); err != nil {
			catalogError(w, err)
			return
		}
		items = append(items, map[string]any{"id": id, "supplier_id": supplier, "supplier_name": name, "payment_reference": ref, "payment_date": date, "amount_minor": amount, "allocated_minor": allocated, "unallocated_minor": amount - allocated, "currency": strings.TrimSpace(currency), "payment_method": method, "external_reference": external, "created_at": created})
	}
	if rows.Err() != nil {
		catalogError(w, rows.Err())
		return
	}
	jsonOut(w, 200, map[string]any{"items": items, "limit": limit, "offset": offset})
}

func (s *server) submitSupplierInvoiceForReview(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	id := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "invoiceID")))
	if !validUUID(id) {
		jsonOut(w, 400, map[string]string{"error": "invalid_invoice_id"})
		return
	}
	var status string
	err := tx.QueryRow(r.Context(), `SELECT status FROM supplier_invoices WHERE business_id=$1 AND branch_id=$2 AND id=$3 FOR UPDATE`, tc.BusinessID, tc.BranchID, id).Scan(&status)
	if err == pgx.ErrNoRows {
		jsonOut(w, 404, map[string]string{"error": "supplier_invoice_not_found"})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	if status == "void" {
		jsonOut(w, 409, map[string]string{"error": "void_invoice_cannot_be_submitted"})
		return
	}
	var latest string
	err = tx.QueryRow(r.Context(), `SELECT event_type FROM supplier_invoice_review_events WHERE business_id=$1 AND invoice_id=$2 ORDER BY event_no DESC LIMIT 1`, tc.BusinessID, id).Scan(&latest)
	if err != nil && err != pgx.ErrNoRows {
		catalogError(w, err)
		return
	}
	if latest == "submitted" || latest == "approved" || latest == "approved_with_exceptions" || latest == "rejected" {
		jsonOut(w, 409, map[string]string{"error": "invoice_not_eligible_for_resubmission", "review_status": latest})
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO supplier_invoice_review_events(business_id,branch_id,invoice_id,event_no,event_type,actor_user_id) VALUES($1,$2,$3,COALESCE((SELECT MAX(event_no)+1 FROM supplier_invoice_review_events WHERE business_id=$1 AND invoice_id=$3),1),'submitted',$4)`, tc.BusinessID, tc.BranchID, id, tc.UserID)
	if err != nil {
		catalogError(w, err)
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary,metadata) VALUES($1,$2,'payables.invoice_submitted','supplier_invoice',$3,'Supplier invoice submitted for approval','{}'::jsonb)`, tc.BusinessID, tc.UserID, id)
	if err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]string{"invoice_id": id, "review_status": "submitted"})
}

func (s *server) reviewSupplierInvoice(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	id := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "invoiceID")))
	if !validUUID(id) {
		jsonOut(w, 400, map[string]string{"error": "invalid_invoice_id"})
		return
	}
	var in supplierInvoiceReviewInput
	if !decode(w, r, &in) {
		return
	}
	in.Decision = strings.TrimSpace(in.Decision)
	in.Reason = strings.TrimSpace(in.Reason)
	if len(in.Reason) > 2000 {
		jsonOut(w, 400, map[string]string{"error": "invalid_review_reason"})
		return
	}
	var status, creator string
	var purchaseOrderID *string
	err := tx.QueryRow(r.Context(), `SELECT status,created_by,purchase_order_id FROM supplier_invoices WHERE business_id=$1 AND branch_id=$2 AND id=$3 FOR UPDATE`, tc.BusinessID, tc.BranchID, id).Scan(&status, &creator, &purchaseOrderID)
	if err == pgx.ErrNoRows {
		jsonOut(w, 404, map[string]string{"error": "supplier_invoice_not_found"})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	if status == "void" {
		jsonOut(w, 409, map[string]string{"error": "void_invoice_cannot_be_reviewed"})
		return
	}
	if creator == tc.UserID {
		jsonOut(w, 403, map[string]string{"error": "invoice_creator_cannot_approve_own_invoice"})
		return
	}
	// Serialize approval against receiving on the linked purchase order so the
	// matching check cannot approve against a receipt snapshot being changed.
	if purchaseOrderID != nil {
		var lockedPO string
		err = tx.QueryRow(r.Context(), `SELECT id FROM purchase_orders WHERE business_id=$1 AND branch_id=$2 AND id=$3 FOR UPDATE`, tc.BusinessID, tc.BranchID, *purchaseOrderID).Scan(&lockedPO)
		if err == pgx.ErrNoRows {
			jsonOut(w, 409, map[string]string{"error": "linked_purchase_order_not_found"})
			return
		}
		if err != nil {
			catalogError(w, err)
			return
		}
	}
	var latest string
	err = tx.QueryRow(r.Context(), `SELECT event_type FROM supplier_invoice_review_events WHERE business_id=$1 AND invoice_id=$2 ORDER BY event_no DESC LIMIT 1`, tc.BusinessID, id).Scan(&latest)
	if err != nil && err != pgx.ErrNoRows {
		catalogError(w, err)
		return
	}
	if err == pgx.ErrNoRows || latest != "submitted" {
		jsonOut(w, 409, map[string]string{"error": "invoice_not_awaiting_review", "review_status": latest})
		return
	}
	var hasExceptions bool
	err = tx.QueryRow(r.Context(), `SELECT EXISTS (
		SELECT 1 FROM supplier_invoice_lines il
		JOIN supplier_invoices i ON i.business_id=il.business_id AND i.id=il.invoice_id
		LEFT JOIN purchase_order_lines pol ON pol.business_id=il.business_id AND pol.purchase_order_id=i.purchase_order_id AND pol.id=il.purchase_order_line_id
		LEFT JOIN LATERAL (
			SELECT SUM(prl.received_quantity)::numeric(20,8) AS received_quantity,
				ROUND(SUM(prl.received_quantity*prl.unit_cost_minor)/NULLIF(SUM(prl.received_quantity),0))::bigint AS weighted_unit_cost_minor
			FROM purchase_receipt_lines prl JOIN purchase_receipts pr ON pr.business_id=prl.business_id AND pr.id=prl.receipt_id
			WHERE prl.business_id=il.business_id AND prl.purchase_order_line_id=il.purchase_order_line_id AND pr.purchase_order_id=i.purchase_order_id AND pr.branch_id=i.branch_id
		) rec ON TRUE
		WHERE il.business_id=$1 AND il.invoice_id=$2 AND (
			i.purchase_order_id IS NULL OR pol.id IS NULL OR COALESCE(rec.received_quantity,0)=0 OR
			il.quantity>COALESCE(rec.received_quantity,0) OR il.quantity>pol.ordered_quantity OR
			il.unit_cost_minor<>pol.unit_cost_minor OR (rec.weighted_unit_cost_minor IS NOT NULL AND il.unit_cost_minor<>rec.weighted_unit_cost_minor)
		)
	)`, tc.BusinessID, id).Scan(&hasExceptions)
	if err != nil {
		catalogError(w, err)
		return
	}
	if code := validateInvoiceReviewDecision(in.Decision, in.Reason, hasExceptions); code != "" {
		statusCode := 422
		if code == "invalid_review_decision" {
			statusCode = 400
		}
		jsonOut(w, statusCode, map[string]string{"error": code})
		return
	}
	eventType := map[string]string{"approve": "approved", "approve_with_exceptions": "approved_with_exceptions", "reject": "rejected"}[in.Decision]
	_, err = tx.Exec(r.Context(), `INSERT INTO supplier_invoice_review_events(business_id,branch_id,invoice_id,event_no,event_type,reason,actor_user_id) VALUES($1,$2,$3,COALESCE((SELECT MAX(event_no)+1 FROM supplier_invoice_review_events WHERE business_id=$1 AND invoice_id=$3),1),$4,NULLIF($5,''),$6)`, tc.BusinessID, tc.BranchID, id, eventType, in.Reason, tc.UserID)
	if err != nil {
		catalogError(w, err)
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary,metadata) VALUES($1,$2,$3,'supplier_invoice',$4,'Supplier invoice review decision',jsonb_build_object('decision',$5,'reason',$6,'matching_exceptions',$7))`, tc.BusinessID, tc.UserID, "payables.invoice_"+eventType, id, in.Decision, in.Reason, hasExceptions)
	if err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"invoice_id": id, "review_status": eventType, "matching_exceptions": hasExceptions})
}

func (s *server) listSupplierInvoiceReviewEvents(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	id := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "invoiceID")))
	if !validUUID(id) {
		jsonOut(w, 400, map[string]string{"error": "invalid_invoice_id"})
		return
	}
	var exists bool
	err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM supplier_invoices WHERE business_id=$1 AND branch_id=$2 AND id=$3)`, tc.BusinessID, tc.BranchID, id).Scan(&exists)
	if err != nil {
		catalogError(w, err)
		return
	}
	if !exists {
		jsonOut(w, 404, map[string]string{"error": "supplier_invoice_not_found"})
		return
	}
	rows, err := tx.Query(r.Context(), `SELECT e.event_type,e.reason,e.actor_user_id,e.created_at FROM supplier_invoice_review_events e WHERE e.business_id=$1 AND e.branch_id=$2 AND e.invoice_id=$3 ORDER BY e.event_no`, tc.BusinessID, tc.BranchID, id)
	if err != nil {
		catalogError(w, err)
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var event string
		var reason *string
		var actor string
		var created any
		if err = rows.Scan(&event, &reason, &actor, &created); err != nil {
			catalogError(w, err)
			return
		}
		items = append(items, map[string]any{"event_type": event, "reason": reason, "actor_user_id": actor, "created_at": created})
	}
	if rows.Err() != nil {
		catalogError(w, rows.Err())
		return
	}
	jsonOut(w, 200, map[string]any{"items": items})
}

func effectiveInvoiceStatus(stored string, paid, total int64) string {
	if stored == "void" {
		return "void"
	}
	if paid >= total {
		return "paid"
	}
	if paid > 0 {
		return "partially_paid"
	}
	return stored
}

func maxInt64(value, minimum int64) int64 {
	if value < minimum {
		return minimum
	}
	return value
}

// getSupplierInvoiceMatch provides a read-only three-way comparison between
// invoice lines, their purchase-order lines, and posted receipt lines. It does
// not approve, post, or mutate an invoice or inventory record.
func (s *server) getSupplierInvoiceMatch(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	invoiceID := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "invoiceID")))
	if !validUUID(invoiceID) {
		jsonOut(w, 400, map[string]string{"error": "invalid_invoice_id"})
		return
	}
	var supplierName, invoiceNumber, currency string
	var purchaseOrderID *string
	var totalMinor int64
	err := tx.QueryRow(r.Context(), `SELECT s.name,i.invoice_number,i.currency,i.total_minor,i.purchase_order_id
		FROM supplier_invoices i JOIN suppliers s ON s.business_id=i.business_id AND s.id=i.supplier_id
		WHERE i.business_id=$1 AND i.branch_id=$2 AND i.id=$3`, tc.BusinessID, tc.BranchID, invoiceID).
		Scan(&supplierName, &invoiceNumber, &currency, &totalMinor, &purchaseOrderID)
	if err == pgx.ErrNoRows {
		jsonOut(w, 404, map[string]string{"error": "supplier_invoice_not_found"})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	var purchaseOrderArg any
	if purchaseOrderID != nil {
		purchaseOrderArg = *purchaseOrderID
	}
	rows, err := tx.Query(r.Context(), `
		SELECT il.id,il.product_id,p.name,il.description,il.quantity::text,
		       il.unit_cost_minor,il.line_total_minor,il.purchase_order_line_id,
		       pol.ordered_quantity::text,pol.unit_cost_minor,
		       COALESCE(receipts.received_quantity,0)::text,receipts.weighted_unit_cost_minor
		FROM supplier_invoice_lines il
		JOIN products p ON p.business_id=il.business_id AND p.id=il.product_id
		LEFT JOIN purchase_order_lines pol
		  ON pol.business_id=il.business_id AND pol.purchase_order_id=$2 AND pol.id=il.purchase_order_line_id
		LEFT JOIN LATERAL (
		  SELECT SUM(prl.received_quantity)::numeric(20,8) AS received_quantity,
		         ROUND(SUM(prl.received_quantity*prl.unit_cost_minor)/NULLIF(SUM(prl.received_quantity),0))::bigint AS weighted_unit_cost_minor
		  FROM purchase_receipt_lines prl
		  JOIN purchase_receipts pr ON pr.business_id=prl.business_id AND pr.id=prl.receipt_id
		  WHERE prl.business_id=il.business_id AND prl.purchase_order_line_id=il.purchase_order_line_id
		    AND pr.purchase_order_id=$2 AND pr.branch_id=$3
		) receipts ON TRUE
		WHERE il.business_id=$1 AND il.invoice_id=$4
		ORDER BY il.created_at,il.id`, tc.BusinessID, purchaseOrderArg, tc.BranchID, invoiceID)
	if err != nil {
		catalogError(w, err)
		return
	}
	defer rows.Close()
	lines := make([]map[string]any, 0)
	matchedCount, varianceCount := 0, 0
	for rows.Next() {
		var lineID, productID, productName, description, invoiceQty, receivedQty string
		var poQty sql.NullString
		var invoiceUnitCost, invoiceLineTotal int64
		var poLineID *string
		var poUnitCost, receiptUnitCost *int64
		if err = rows.Scan(&lineID, &productID, &productName, &description, &invoiceQty,
			&invoiceUnitCost, &invoiceLineTotal, &poLineID, &poQty, &poUnitCost, &receivedQty, &receiptUnitCost); err != nil {
			catalogError(w, err)
			return
		}
		flags := make([]string, 0, 5)
		if purchaseOrderID == nil || poLineID == nil || !poQty.Valid {
			flags = append(flags, "unlinked_purchase_order_line")
		} else {
			invQ, invOK := new(big.Rat).SetString(invoiceQty)
			orderedQ, orderedOK := new(big.Rat).SetString(poQty.String)
			receivedQ, receivedOK := new(big.Rat).SetString(receivedQty)
			if !invOK || !orderedOK || !receivedOK {
				jsonOut(w, 500, map[string]string{"error": "invalid_quantity_in_match_records"})
				return
			}
			if receivedQ.Sign() == 0 {
				flags = append(flags, "no_goods_received")
			} else if invQ.Cmp(receivedQ) > 0 {
				flags = append(flags, "invoice_quantity_exceeds_received")
			}
			if invQ.Cmp(orderedQ) > 0 {
				flags = append(flags, "invoice_quantity_exceeds_ordered")
			}
			if poUnitCost != nil && invoiceUnitCost != *poUnitCost {
				flags = append(flags, "purchase_order_price_variance")
			}
			if receiptUnitCost != nil && invoiceUnitCost != *receiptUnitCost {
				flags = append(flags, "receipt_cost_variance")
			}
		}
		status := "matched"
		if len(flags) > 0 {
			status = "exception"
			varianceCount++
		} else {
			matchedCount++
		}
		lines = append(lines, map[string]any{
			"invoice_line_id": lineID, "product_id": productID, "product_name": productName,
			"description": description, "invoice_quantity": invoiceQty,
			"invoice_unit_cost_minor": invoiceUnitCost, "invoice_line_total_minor": invoiceLineTotal,
			"purchase_order_line_id": poLineID, "ordered_quantity": nullableString(poQty.String),
			"purchase_order_unit_cost_minor": poUnitCost, "received_quantity": receivedQty,
			"receipt_weighted_unit_cost_minor": receiptUnitCost, "status": status, "exceptions": flags,
		})
	}
	if rows.Err() != nil {
		catalogError(w, rows.Err())
		return
	}
	jsonOut(w, 200, map[string]any{
		"invoice_id": invoiceID, "invoice_number": invoiceNumber, "supplier_name": supplierName,
		"purchase_order_id": purchaseOrderID, "currency": currency, "total_minor": totalMinor,
		"line_count": len(lines), "matched_line_count": matchedCount,
		"exception_line_count": varianceCount, "status": map[bool]string{true: "exceptions", false: "matched"}[varianceCount > 0],
		"lines": lines,
	})
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
