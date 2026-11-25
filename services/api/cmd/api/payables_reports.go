package main

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func payablesAgingBucket(days int) string {
	switch {
	case days <= 0:
		return "current"
	case days <= 30:
		return "1_30"
	case days <= 60:
		return "31_60"
	case days <= 90:
		return "61_90"
	default:
		return "over_90"
	}
}

func parsePayablesDateRange(r *http.Request) (string, string, bool) {
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	if !validISODate(from, true) || !validISODate(to, true) {
		return "", "", false
	}
	if from != "" && to != "" && from > to {
		return "", "", false
	}
	return from, to, true
}

// getSupplierStatement returns a supplier-level subledger for one currency.
// Payments are credited in full; their allocated and unapplied portions remain
// visible so that a payment is never mistaken for a bank reconciliation.
func (s *server) getSupplierStatement(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	supplierID := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "supplierID")))
	currency := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("currency")))
	from, to, valid := parsePayablesDateRange(r)
	if !validUUID(supplierID) || !validCurrencyCode(currency) || !valid {
		jsonOut(w, 400, map[string]string{"error": "invalid_supplier_statement_filters"})
		return
	}
	var supplierName string
	if err := tx.QueryRow(r.Context(), `SELECT name FROM suppliers WHERE business_id=$1 AND id=$2`, tc.BusinessID, supplierID).Scan(&supplierName); err != nil {
		if err == pgx.ErrNoRows {
			jsonOut(w, 404, map[string]string{"error": "supplier_not_found"})
			return
		}
		catalogError(w, err)
		return
	}
	rows, err := tx.Query(r.Context(), `
		SELECT event_date::text,event_type,reference,description,debit_minor,credit_minor,allocated_minor,created_at,source_id
		FROM (
			SELECT i.invoice_date AS event_date,'invoice'::text AS event_type,i.invoice_number AS reference,
				'Supplier invoice'::text AS description,i.total_minor AS debit_minor,0::bigint AS credit_minor,
				0::bigint AS allocated_minor,i.created_at,i.id::text AS source_id
			FROM supplier_invoices i
			WHERE i.business_id=$1 AND i.branch_id=$2 AND i.supplier_id=$3 AND btrim(i.currency)=$4 AND i.status<>'void'
			UNION ALL
			SELECT p.payment_date,'payment'::text,p.payment_reference,
				('Supplier payment · '||p.payment_method)::text,0::bigint,p.amount_minor,
				COALESCE(a.allocated_minor,0)::bigint,p.created_at,p.id::text
			FROM supplier_payments p
			LEFT JOIN LATERAL (SELECT sum(pa.amount_minor)::bigint AS allocated_minor FROM supplier_payment_allocations pa WHERE pa.business_id=p.business_id AND pa.payment_id=p.id) a ON TRUE
			WHERE p.business_id=$1 AND p.branch_id=$2 AND p.supplier_id=$3 AND btrim(p.currency)=$4
		) ledger
		WHERE NULLIF($5::text,'') IS NULL OR event_date <= NULLIF($5::text,'')::date
		ORDER BY event_date,CASE WHEN event_type='invoice' THEN 0 ELSE 1 END,created_at,source_id`,
		tc.BusinessID, tc.BranchID, supplierID, currency, to)
	if err != nil {
		catalogError(w, err)
		return
	}
	defer rows.Close()
	lines := []map[string]any{}
	var debitTotal, creditTotal, allocatedTotal, unappliedTotal, running, opening int64
	for rows.Next() {
		var date, kind, reference, description, sourceID string
		var debit, credit, allocated int64
		var created time.Time
		if err = rows.Scan(&date, &kind, &reference, &description, &debit, &credit, &allocated, &created, &sourceID); err != nil {
			catalogError(w, err)
			return
		}
		if from != "" && date < from {
			opening += debit - credit
			continue
		}
		running += debit - credit
		debitTotal += debit
		creditTotal += credit
		allocatedTotal += allocated
		unapplied := int64(0)
		if kind == "payment" {
			unapplied = credit - allocated
			unappliedTotal += unapplied
		}
		lines = append(lines, map[string]any{"date": date, "type": kind, "reference": reference, "description": description, "debit_minor": debit, "credit_minor": credit, "allocated_minor": allocated, "unapplied_minor": unapplied, "running_balance_minor": opening + running, "source_id": sourceID})
	}
	if rows.Err() != nil {
		catalogError(w, rows.Err())
		return
	}
	jsonOut(w, 200, map[string]any{"supplier_id": supplierID, "supplier_name": supplierName, "currency": currency, "from": from, "to": to, "opening_balance_minor": opening, "debit_total_minor": debitTotal, "credit_total_minor": creditTotal, "allocated_payment_minor": allocatedTotal, "unapplied_payment_minor": unappliedTotal, "closing_balance_minor": opening + running, "lines": lines, "basis": "branch supplier subledger; payments are credited in full and unapplied amounts are disclosed"})
}

func (s *server) getSupplierAgingReport(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	asOf := strings.TrimSpace(r.URL.Query().Get("as_of"))
	if !validISODate(asOf, true) {
		jsonOut(w, 400, map[string]string{"error": "invalid_as_of_date"})
		return
	}
	var effectiveAsOf string
	if err := tx.QueryRow(r.Context(), `SELECT COALESCE(NULLIF($1::text,'' )::date,CURRENT_DATE)::text`, asOf).Scan(&effectiveAsOf); err != nil {
		catalogError(w, err)
		return
	}
	rows, err := tx.Query(r.Context(), `
		WITH allocated AS (
			SELECT business_id,invoice_id,sum(amount_minor)::bigint AS amount_minor
			FROM supplier_payment_allocations WHERE business_id=$1 GROUP BY business_id,invoice_id
		), open_invoices AS (
			SELECT i.supplier_id,s.name AS supplier_name,btrim(i.currency) AS currency,
				GREATEST(i.total_minor-COALESCE(a.amount_minor,0),0)::bigint AS outstanding_minor,
				(COALESCE(NULLIF($3::text,'')::date,CURRENT_DATE)-COALESCE(i.due_date,i.invoice_date))::int AS age_days
			FROM supplier_invoices i
			JOIN suppliers s ON s.business_id=i.business_id AND s.id=i.supplier_id
			LEFT JOIN allocated a ON a.business_id=i.business_id AND a.invoice_id=i.id
			WHERE i.business_id=$1 AND i.branch_id=$2 AND i.status<>'void' AND i.total_minor>COALESCE(a.amount_minor,0)
		)
		SELECT supplier_id,supplier_name,currency,
			COALESCE(sum(outstanding_minor) FILTER (WHERE age_days<=0),0)::bigint AS current_minor,
			COALESCE(sum(outstanding_minor) FILTER (WHERE age_days BETWEEN 1 AND 30),0)::bigint AS days_1_30_minor,
			COALESCE(sum(outstanding_minor) FILTER (WHERE age_days BETWEEN 31 AND 60),0)::bigint AS days_31_60_minor,
			COALESCE(sum(outstanding_minor) FILTER (WHERE age_days BETWEEN 61 AND 90),0)::bigint AS days_61_90_minor,
			COALESCE(sum(outstanding_minor) FILTER (WHERE age_days>90),0)::bigint AS over_90_minor,
			COALESCE(sum(outstanding_minor),0)::bigint AS total_outstanding_minor,
			count(*)::int AS invoice_count
		FROM open_invoices GROUP BY supplier_id,supplier_name,currency
		ORDER BY supplier_name,currency`, tc.BusinessID, tc.BranchID, asOf)
	if err != nil {
		catalogError(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, name, currency string
		var current, d1, d31, d61, over90, total int64
		var count int
		if err = rows.Scan(&id, &name, &currency, &current, &d1, &d31, &d61, &over90, &total, &count); err != nil {
			catalogError(w, err)
			return
		}
		items = append(items, map[string]any{"supplier_id": id, "supplier_name": name, "currency": currency, "current_minor": current, "days_1_30_minor": d1, "days_31_60_minor": d31, "days_61_90_minor": d61, "over_90_minor": over90, "total_outstanding_minor": total, "invoice_count": count})
	}
	if rows.Err() != nil {
		catalogError(w, rows.Err())
		return
	}
	jsonOut(w, 200, map[string]any{"as_of": effectiveAsOf, "items": items})
}

func (s *server) getPaymentAllocationReconciliation(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	from, to, valid := parsePayablesDateRange(r)
	if !valid {
		jsonOut(w, 400, map[string]string{"error": "invalid_date_range"})
		return
	}
	rows, err := tx.Query(r.Context(), `
		SELECT p.id,p.supplier_id,s.name,p.payment_reference,p.payment_date::text,btrim(p.currency),
			p.amount_minor,COALESCE(a.allocated_minor,0)::bigint,(p.amount_minor-COALESCE(a.allocated_minor,0))::bigint,
			COALESCE(a.allocation_count,0)::int
		FROM supplier_payments p
		JOIN suppliers s ON s.business_id=p.business_id AND s.id=p.supplier_id
		LEFT JOIN LATERAL (SELECT sum(pa.amount_minor)::bigint AS allocated_minor,count(*)::int AS allocation_count FROM supplier_payment_allocations pa WHERE pa.business_id=p.business_id AND pa.payment_id=p.id) a ON TRUE
		WHERE p.business_id=$1 AND p.branch_id=$2 AND ($3='' OR p.payment_date >= $3::date) AND ($4='' OR p.payment_date <= $4::date)
		ORDER BY p.payment_date DESC,p.created_at DESC,p.id`, tc.BusinessID, tc.BranchID, from, to)
	if err != nil {
		catalogError(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	totalsByCurrency := map[string]map[string]int64{}
	for rows.Next() {
		var id, supplierID, supplier, reference, date, currency string
		var amount, allocated, unapplied int64
		var count int
		if err = rows.Scan(&id, &supplierID, &supplier, &reference, &date, &currency, &amount, &allocated, &unapplied, &count); err != nil {
			catalogError(w, err)
			return
		}
		if totalsByCurrency[currency] == nil {
			totalsByCurrency[currency] = map[string]int64{"payment_total_minor": 0, "allocated_total_minor": 0, "unapplied_total_minor": 0}
		}
		totalsByCurrency[currency]["payment_total_minor"] += amount
		totalsByCurrency[currency]["allocated_total_minor"] += allocated
		totalsByCurrency[currency]["unapplied_total_minor"] += unapplied
		items = append(items, map[string]any{"payment_id": id, "supplier_id": supplierID, "supplier_name": supplier, "payment_reference": reference, "payment_date": date, "currency": currency, "amount_minor": amount, "allocated_minor": allocated, "unapplied_minor": unapplied, "allocation_count": count, "reconciled": unapplied == 0})
	}
	if rows.Err() != nil {
		catalogError(w, rows.Err())
		return
	}
	jsonOut(w, 200, map[string]any{"from": from, "to": to, "totals_by_currency": totalsByCurrency, "payment_count": len(items), "items": items, "basis": "internal payment-to-invoice allocation reconciliation; not bank statement reconciliation"})
}
