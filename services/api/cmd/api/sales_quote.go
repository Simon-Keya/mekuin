package main

import (
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

type saleQuoteInputLine struct {
	ProductID string `json:"product_id"`
	Quantity  int64  `json:"quantity"`
}
type saleQuoteRequest struct {
	Lines []saleQuoteInputLine `json:"lines"`
}

// quoteSale recalculates every price from the tenant catalogue. Client-supplied
// prices and totals are never accepted. Tax, service charge and discounts remain
// explicit zeroes pending approved business policy.
func (s *server) quoteSale(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, http.StatusInternalServerError, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in saleQuoteRequest
	if !decode(w, r, &in) {
		return
	}
	if len(in.Lines) == 0 || len(in.Lines) > 100 {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "sale_requires_1_to_100_lines"})
		return
	}
	lines := make([]saleQuoteLine, 0, len(in.Lines))
	seen := make(map[string]bool, len(in.Lines))
	for _, requested := range in.Lines {
		id := strings.ToLower(strings.TrimSpace(requested.ProductID))
		if !validUUID(id) || seen[id] || requested.Quantity <= 0 || requested.Quantity > 10000 {
			jsonOut(w, http.StatusBadRequest, map[string]string{"error": "invalid_or_duplicate_sale_line"})
			return
		}
		seen[id] = true
		var sku, name string
		var price int64
		err := tx.QueryRow(r.Context(), `SELECT sku,name,sale_price_minor FROM products WHERE business_id=$1 AND is_active=true AND is_sellable=true AND id=$2`, tc.BusinessID, id).Scan(&sku, &name, &price)
		if err == pgx.ErrNoRows {
			jsonOut(w, http.StatusUnprocessableEntity, map[string]string{"error": "product_not_sellable"})
			return
		}
		if err != nil {
			catalogError(w, err)
			return
		}
		line, err := calculateSaleLine(id, sku, name, requested.Quantity, price)
		if err != nil {
			jsonOut(w, http.StatusUnprocessableEntity, map[string]string{"error": "sale_line_amount_invalid"})
			return
		}
		lines = append(lines, line)
	}
	var currency string
	if err := tx.QueryRow(r.Context(), `SELECT currency FROM businesses WHERE id=$1`, tc.BusinessID).Scan(&currency); err != nil {
		catalogError(w, err)
		return
	}
	totals, err := calculateSaleTotals(lines)
	if err != nil {
		jsonOut(w, http.StatusUnprocessableEntity, map[string]string{"error": "sale_total_invalid"})
		return
	}
	jsonOut(w, http.StatusOK, map[string]any{"currency": currency, "lines": lines, "totals": totals, "calculation_policy": "catalogue_prices_integer_minor_units; tax_service_charge_and_discounts_not_applied_pending_business_policy"})
}
