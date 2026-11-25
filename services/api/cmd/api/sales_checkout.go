package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
)

type checkoutTender struct {
	Method string `json:"method"`
	Amount int64  `json:"amount_minor"`
	Phone  string `json:"phone_e164,omitempty"`
}

type checkoutRequest struct {
	Lines         []saleQuoteInputLine `json:"lines"`
	OrderID       string               `json:"order_id,omitempty"`
	PaymentMethod string               `json:"payment_method,omitempty"` // legacy single-tender form
	Payments      []checkoutTender     `json:"payments,omitempty"`
}

// checkoutSale persists server-priced sale snapshots. Cash completes immediately;
// a single full-value M-Pesa tender remains pending until provider reconciliation
// and the separate atomic settlement endpoint succeed.
func (s *server) checkoutSale(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(key) < 8 || len(key) > 64 {
		jsonOut(w, 400, map[string]string{"error": "valid_idempotency_key_required"})
		return
	}
	var in checkoutRequest
	if !decode(w, r, &in) {
		return
	}
	if in.OrderID != "" {
		in.OrderID = strings.ToLower(strings.TrimSpace(in.OrderID))
		if !validUUID(in.OrderID) {
			jsonOut(w, 400, map[string]string{"error": "invalid_order_id"})
			return
		}
		var orderStatus string
		err := tx.QueryRow(r.Context(), `SELECT status FROM pos_orders WHERE business_id=$1 AND branch_id=$2 AND id=$3 FOR UPDATE`, tc.BusinessID, tc.BranchID, in.OrderID).Scan(&orderStatus)
		if err == pgx.ErrNoRows {
			jsonOut(w, 404, map[string]string{"error": "order_not_found"})
			return
		}
		if err != nil {
			catalogError(w, err)
			return
		}
		if orderStatus != "open" {
			jsonOut(w, 409, map[string]string{"error": "order_must_be_open_for_checkout"})
			return
		}
		orderRows, qerr := tx.Query(r.Context(), `SELECT product_id::text,quantity FROM pos_order_lines WHERE business_id=$1 AND order_id=$2 ORDER BY created_at,id`, tc.BusinessID, in.OrderID)
		if qerr != nil {
			catalogError(w, qerr)
			return
		}
		in.Lines = make([]saleQuoteInputLine, 0)
		for orderRows.Next() {
			var line saleQuoteInputLine
			if qerr = orderRows.Scan(&line.ProductID, &line.Quantity); qerr != nil {
				orderRows.Close()
				catalogError(w, qerr)
				return
			}
			in.Lines = append(in.Lines, line)
		}
		qerr = orderRows.Err()
		orderRows.Close()
		if qerr != nil {
			catalogError(w, qerr)
			return
		}
	}
	if len(in.Lines) == 0 || len(in.Lines) > 100 {
		jsonOut(w, 400, map[string]string{"error": "sale_requires_1_to_100_lines"})
		return
	}
	raw, err := json.Marshal(in)
	if err != nil {
		jsonOut(w, 400, map[string]string{"error": "invalid_checkout_request"})
		return
	}
	sum := sha256.Sum256(raw)
	fingerprint := hex.EncodeToString(sum[:])
	// Serialize identical keys in the tenant/branch scope so concurrent replays
	// observe the first committed sale instead of creating duplicate records.
	lockKey := tc.BusinessID + ":" + tc.BranchID + ":" + key
	if _, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, lockKey); err != nil {
		catalogError(w, err)
		return
	}
	var existingSale, existingHash string
	err = tx.QueryRow(r.Context(), `SELECT sale_id::text,request_hash FROM checkout_idempotency WHERE business_id=$1 AND branch_id=$2 AND idempotency_key=$3`, tc.BusinessID, tc.BranchID, key).Scan(&existingSale, &existingHash)
	if err == nil {
		if existingHash != fingerprint {
			jsonOut(w, 409, map[string]string{"error": "idempotency_key_reused_with_different_request"})
			return
		}
		var total int64
		var status string
		if err = tx.QueryRow(r.Context(), `SELECT total_minor,status FROM sales WHERE business_id=$1 AND id=$2`, tc.BusinessID, existingSale).Scan(&total, &status); err != nil {
			catalogError(w, err)
			return
		}
		replay := map[string]any{"sale_id": existingSale, "total_minor": total, "status": status, "idempotent_replay": true}
		if status == "pending_payment" {
			var attemptID, attemptStatus string
			err = tx.QueryRow(r.Context(), `SELECT id::text,status FROM mpesa_payment_attempts WHERE business_id=$1 AND sale_id=$2`, tc.BusinessID, existingSale).Scan(&attemptID, &attemptStatus)
			if err == nil {
				replay["attempt_id"], replay["provider_attempt_status"] = attemptID, attemptStatus
			} else if !errors.Is(err, pgx.ErrNoRows) {
				catalogError(w, err)
				return
			}
		}
		jsonOut(w, 200, replay)
		return
	}
	if err != pgx.ErrNoRows {
		catalogError(w, err)
		return
	}
	lines := make([]saleQuoteLine, 0, len(in.Lines))
	seen := map[string]bool{}
	for _, requested := range in.Lines {
		id := strings.ToLower(strings.TrimSpace(requested.ProductID))
		if !validUUID(id) || seen[id] || requested.Quantity <= 0 || requested.Quantity > 10000 {
			jsonOut(w, 400, map[string]string{"error": "invalid_or_duplicate_sale_line"})
			return
		}
		seen[id] = true
		var sku, name string
		var price int64
		err = tx.QueryRow(r.Context(), `SELECT sku,name,sale_price_minor FROM products WHERE business_id=$1 AND is_active=true AND is_sellable=true AND id=$2`, tc.BusinessID, id).Scan(&sku, &name, &price)
		if err == pgx.ErrNoRows {
			jsonOut(w, 422, map[string]string{"error": "product_not_sellable"})
			return
		}
		if err != nil {
			catalogError(w, err)
			return
		}
		line, e := calculateSaleLine(id, sku, name, requested.Quantity, price)
		if e != nil {
			jsonOut(w, 422, map[string]string{"error": "sale_line_amount_invalid"})
			return
		}
		lines = append(lines, line)
	}
	totals, err := calculateSaleTotals(lines)
	if err != nil {
		jsonOut(w, 422, map[string]string{"error": "sale_total_invalid"})
		return
	}
	if totals.Total <= 0 {
		jsonOut(w, 422, map[string]string{"error": "checkout_total_must_be_positive"})
		return
	}
	var currency string
	if err = tx.QueryRow(r.Context(), `SELECT currency FROM businesses WHERE id=$1`, tc.BusinessID).Scan(&currency); err != nil {
		catalogError(w, err)
		return
	}
	tenders, tenderErr := normalizeCheckoutTenders(in, totals.Total)
	if tenderErr != nil {
		jsonOut(w, http.StatusUnprocessableEntity, map[string]string{"error": tenderErr.Error()})
		return
	}
	isMPesa := len(tenders) == 1 && tenders[0].Method == "mpesa"
	if isMPesa && !strings.EqualFold(currency, "KES") {
		jsonOut(w, http.StatusUnprocessableEntity, map[string]string{"error": "mpesa_requires_kes_business_currency"})
		return
	}
	if isMPesa && in.OrderID != "" {
		jsonOut(w, http.StatusUnprocessableEntity, map[string]string{"error": "mpesa_order_checkout_not_yet_supported_use_direct_sale_checkout"})
		return
	}
	if isMPesa && (tenders[0].Amount%100 != 0 || tenders[0].Amount/100 > 250000 || !validDarajaPhone(tenders[0].Phone)) {
		jsonOut(w, http.StatusUnprocessableEntity, map[string]string{"error": "mpesa_requires_valid_phone_and_whole_kes_amount_within_provider_limit"})
		return
	}
	for _, tender := range tenders {
		if tender.Method != "cash" && tender.Method != "mpesa" || (tender.Method == "mpesa" && !isMPesa) {
			jsonOut(w, http.StatusUnprocessableEntity, map[string]string{"error": "only_cash_or_single_full_value_mpesa_tender_supported"})
			return
		}
	}
	saleStatus := "completed"
	if isMPesa {
		saleStatus = "pending_payment"
	}
	var saleID string
	saleNumber := "POS-" + tc.BranchID[:8] + "-" + key
	err = tx.QueryRow(r.Context(), `INSERT INTO sales(business_id,branch_id,sale_number,status,total_minor,created_by,source_order_id) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,'')::uuid) RETURNING id::text`, tc.BusinessID, tc.BranchID, saleNumber, saleStatus, totals.Total, tc.UserID, in.OrderID).Scan(&saleID)
	if err != nil {
		catalogError(w, err)
		return
	}
	for _, line := range lines {
		if _, err = tx.Exec(r.Context(), `INSERT INTO sale_lines(business_id,sale_id,product_id,sku_snapshot,name_snapshot,quantity,unit_price_minor,line_total_minor) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, tc.BusinessID, saleID, line.ProductID, line.SKU, line.Name, line.Quantity, line.UnitPrice, line.LineTotal); err != nil {
			catalogError(w, err)
			return
		}
	}
	// Recipe-backed items consume their ingredients in the same transaction as
	// the sale. Products without recipes remain outside automatic stock control
	// until the product stock-tracking policy is explicitly configured.
	consumed := 0
	if !isMPesa {
		consumed, err = applyRecipeStockConsumption(r, tx, tc, saleID, lines)
	}
	if err != nil {
		if err == errInsufficientStock {
			jsonOut(w, http.StatusConflict, map[string]string{"error": "insufficient_stock_for_recipe"})
			return
		}
		if err == errRecipeConversion {
			jsonOut(w, http.StatusUnprocessableEntity, map[string]string{"error": "recipe_unit_conversion_missing_or_inexact"})
			return
		}
		catalogError(w, err)
		return
	}
	paymentStatus := "recorded"
	if isMPesa {
		paymentStatus = "pending_verification"
	}
	var mpesaPaymentID string
	for _, tender := range tenders {
		var cashShiftID any
		if tender.Method == "cash" {
			var shiftID string
			err = tx.QueryRow(r.Context(), `SELECT id::text FROM cash_shifts WHERE business_id=$1 AND branch_id=$2 AND user_id=$3 AND status='open' FOR UPDATE`, tc.BusinessID, tc.BranchID, tc.UserID).Scan(&shiftID)
			if err == pgx.ErrNoRows {
				jsonOut(w, http.StatusConflict, map[string]string{"error": "cash_shift_required"})
				return
			}
			if err != nil {
				catalogError(w, err)
				return
			}
			cashShiftID = shiftID
		}
		if err = tx.QueryRow(r.Context(), `INSERT INTO sale_payments(business_id,branch_id,sale_id,method,amount_minor,status,recorded_by,cash_shift_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text`, tc.BusinessID, tc.BranchID, saleID, tender.Method, tender.Amount, paymentStatus, tc.UserID, cashShiftID).Scan(&mpesaPaymentID); err != nil {
			catalogError(w, err)
			return
		}
	}
	var attemptID string
	providerSubmission := "not_applicable"
	if isMPesa {
		providerSubmission = "accepted"
		var currency string
		if err = tx.QueryRow(r.Context(), `SELECT currency FROM businesses WHERE id=$1`, tc.BusinessID).Scan(&currency); err != nil {
			catalogError(w, err)
			return
		}
		environment := strings.ToLower(strings.TrimSpace(os.Getenv("DARAJA_ENV")))
		if environment == "" {
			environment = "sandbox"
		}
		if environment != "sandbox" && environment != "production" {
			jsonOut(w, http.StatusServiceUnavailable, map[string]string{"error": "daraja_environment_invalid"})
			return
		}
		accountRef := "MEKU" + strings.ToUpper(key[:min(8, len(key))])
		if err = tx.QueryRow(r.Context(), `INSERT INTO mpesa_payment_attempts(business_id,branch_id,sale_id,sale_payment_id,environment,phone_e164,amount_minor,currency,account_reference,status,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'created',$10) RETURNING id::text`, tc.BusinessID, tc.BranchID, saleID, mpesaPaymentID, environment, tenders[0].Phone, tenders[0].Amount, currency, accountRef, tc.UserID).Scan(&attemptID); err != nil {
			catalogError(w, err)
			return
		}
		// The durable dispatch row is committed with the sale and attempt. A
		// separate worker submits STK only after this transaction commits, so a
		// fast provider callback can always correlate to a committed attempt.
		if _, err = tx.Exec(r.Context(), `INSERT INTO mpesa_dispatch_outbox(business_id,branch_id,attempt_id) VALUES($1,$2,$3)`, tc.BusinessID, tc.BranchID, attemptID); err != nil {
			catalogError(w, err)
			return
		}
		providerSubmission = "queued_for_dispatch"

	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO checkout_idempotency(business_id,branch_id,idempotency_key,request_hash,sale_id) VALUES($1,$2,$3,$4,$5)`, tc.BusinessID, tc.BranchID, key, fingerprint, saleID); err != nil {
		catalogError(w, err)
		return
	}
	auditAction, auditSummary := "sales.checked_out", "Sale checked out"
	if isMPesa {
		auditAction, auditSummary = "sales.checkout_started", "M-Pesa sale created pending provider verification"
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary,metadata) VALUES($1,$2,$3,'sale',$4,$5,jsonb_build_object('total_minor',$6,'tender_count',$7,'currency',$8))`, tc.BusinessID, tc.UserID, auditAction, saleID, auditSummary, totals.Total, len(tenders), currency); err != nil {
		catalogError(w, err)
		return
	}
	if in.OrderID != "" && !isMPesa {
		tag, updateErr := tx.Exec(r.Context(), `UPDATE pos_orders SET status='checked_out',checked_out_sale_id=$4,updated_at=now() WHERE business_id=$1 AND branch_id=$2 AND id=$3 AND status='open'`, tc.BusinessID, tc.BranchID, in.OrderID, saleID)
		if updateErr != nil {
			catalogError(w, updateErr)
			return
		}
		if tag.RowsAffected() != 1 {
			jsonOut(w, http.StatusConflict, map[string]string{"error": "order_checkout_conflict"})
			return
		}
		if _, updateErr = tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary,metadata) VALUES($1,$2,'pos.order_checked_out','pos_order',$3,'POS order checked out',jsonb_build_object('sale_id',$4))`, tc.BusinessID, tc.UserID, in.OrderID, saleID); updateErr != nil {
			catalogError(w, updateErr)
			return
		}
	}
	response := map[string]any{"sale_id": saleID, "sale_number": saleNumber, "order_id": in.OrderID, "currency": currency, "lines": lines, "totals": totals, "payment_status": paymentStatus, "payments": tenders, "status": saleStatus, "idempotent_replay": false, "stock_deduction": map[string]any{"status": "recipe_and_tracked_direct_items_deducted", "ingredient_movements": consumed, "non_recipe_products": "deducted_when_branch_stock_history_exists_otherwise_untracked"}}
	if isMPesa {
		response["attempt_id"] = attemptID
		response["provider_submission"] = providerSubmission
		response["stock_deduction"] = map[string]any{"status": "deferred_until_verified_settlement"}
		jsonOut(w, http.StatusAccepted, response)
		return
	}
	jsonOut(w, http.StatusCreated, response)
}

var errInsufficientStock = errors.New("insufficient stock")
var errRecipeConversion = errors.New("recipe conversion missing or inexact")

// applyRecipeStockConsumption writes immutable negative movements for recipe
// ingredients. It uses the same directional conversion rule as recipe costing,
// locks each ingredient balance, and refuses negative or >8dp quantities.
func applyRecipeStockConsumption(r *http.Request, tx pgx.Tx, tc tenantContext, saleID string, lines []saleQuoteLine) (int, error) {
	movementCount := 0
	for _, sold := range lines {
		var recipeID, outputQuantity string
		err := tx.QueryRow(r.Context(), `SELECT id::text,output_quantity::text FROM recipes WHERE business_id=$1 AND output_product_id=$2 AND is_active=true FOR SHARE`, tc.BusinessID, sold.ProductID).Scan(&recipeID, &outputQuantity)
		if err == pgx.ErrNoRows {
			// Direct-sale products are inventory-tracked when the branch has an
			// opening or movement history. Catalog-only products without stock
			// history remain explicitly untracked rather than inventing stock.
			var tracked bool
			if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM inventory_movements WHERE business_id=$1 AND branch_id=$2 AND product_id=$3)`, tc.BusinessID, tc.BranchID, sold.ProductID).Scan(&tracked); err != nil {
				return movementCount, err
			}
			if !tracked {
				continue
			}
			if err = lockInventoryBalance(r.Context(), tx, tc.BusinessID, tc.BranchID, sold.ProductID); err != nil {
				return movementCount, err
			}
			var available string
			if err = tx.QueryRow(r.Context(), `SELECT COALESCE(sum(quantity_delta),0)::text FROM inventory_movements WHERE business_id=$1 AND branch_id=$2 AND product_id=$3`, tc.BusinessID, tc.BranchID, sold.ProductID).Scan(&available); err != nil {
				return movementCount, err
			}
			var enough bool
			if err = tx.QueryRow(r.Context(), `SELECT $1::numeric >= $2::numeric`, available, sold.Quantity).Scan(&enough); err != nil {
				return movementCount, err
			}
			if !enough {
				return movementCount, errInsufficientStock
			}
			if _, err = tx.Exec(r.Context(), `INSERT INTO sale_recipe_consumption_snapshots(business_id,branch_id,sale_id,sold_product_id,ingredient_product_id,sold_quantity,recipe_output_quantity,ingredient_quantity_per_output,consumed_quantity,recipe_snapshot) VALUES($1,$2,$3,$4,$4,$5::numeric,1,1,$5::numeric,jsonb_build_object('basis','direct_stock_item'))`, tc.BusinessID, tc.BranchID, saleID, sold.ProductID, sold.Quantity); err != nil {
				return movementCount, err
			}
			if _, err = tx.Exec(r.Context(), `INSERT INTO inventory_movements(business_id,branch_id,product_id,quantity_delta,movement_type,reference_id,created_by) VALUES($1,$2,$3,-($4::numeric),'sale_consumption',$5,$6)`, tc.BusinessID, tc.BranchID, sold.ProductID, sold.Quantity, saleID, tc.UserID); err != nil {
				return movementCount, err
			}
			movementCount++
			continue
		}
		if err != nil {
			return movementCount, err
		}
		rows, err := tx.Query(r.Context(), `
			SELECT ingredient_product_id::text, sum(quantity_in_base)::text, bool_and(conversion_ok)
			FROM (
			 SELECT ri.ingredient_product_id, ri.quantity * CASE
			   WHEN ri.unit_id=p.base_unit_id THEN 1::numeric ELSE uc.factor END AS quantity_in_base,
			   (ri.unit_id=p.base_unit_id OR uc.factor IS NOT NULL) AS conversion_ok
			 FROM recipe_ingredients ri
			 JOIN products p ON p.business_id=ri.business_id AND p.id=ri.ingredient_product_id AND p.is_active=true
			 LEFT JOIN unit_conversions uc ON uc.business_id=ri.business_id
			  AND uc.from_unit_id=ri.unit_id AND uc.to_unit_id=p.base_unit_id
			 WHERE ri.business_id=$1 AND ri.recipe_id=$2
			) converted GROUP BY ingredient_product_id ORDER BY ingredient_product_id`, tc.BusinessID, recipeID)
		if err != nil {
			return movementCount, err
		}
		type ingredient struct {
			product, qty string
			conversionOK bool
		}
		items := make([]ingredient, 0)
		for rows.Next() {
			var item ingredient
			if err = rows.Scan(&item.product, &item.qty, &item.conversionOK); err != nil {
				rows.Close()
				return movementCount, err
			}
			items = append(items, item)
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return movementCount, err
		}
		rows.Close()
		if len(items) == 0 {
			return movementCount, errors.New("active recipe has no ingredients")
		}
		for _, item := range items {
			if !item.conversionOK {
				return movementCount, errRecipeConversion
			}
			lockKey := tc.BusinessID + ":" + tc.BranchID + ":stock:" + item.product
			if _, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, lockKey); err != nil {
				return movementCount, err
			}
			var required string
			err = tx.QueryRow(r.Context(), `SELECT ($1::numeric * $2::numeric / $3::numeric)::text`, sold.Quantity, item.qty, outputQuantity).Scan(&required)
			if err != nil {
				return movementCount, err
			}
			var exact bool
			if err = tx.QueryRow(r.Context(), `SELECT $1::numeric>0 AND $1::numeric=round($1::numeric,8)`, required).Scan(&exact); err != nil {
				return movementCount, err
			}
			if !exact {
				return movementCount, errRecipeConversion
			}
			var available string
			if err = tx.QueryRow(r.Context(), `SELECT COALESCE(sum(quantity_delta),0)::text FROM inventory_movements WHERE business_id=$1 AND branch_id=$2 AND product_id=$3`, tc.BusinessID, tc.BranchID, item.product).Scan(&available); err != nil {
				return movementCount, err
			}
			var enough bool
			if err = tx.QueryRow(r.Context(), `SELECT $1::numeric >= $2::numeric`, available, required).Scan(&enough); err != nil {
				return movementCount, err
			}
			if !enough {
				return movementCount, errInsufficientStock
			}
			if _, err = tx.Exec(r.Context(), `INSERT INTO sale_recipe_consumption_snapshots(business_id,branch_id,sale_id,sold_product_id,ingredient_product_id,sold_quantity,recipe_output_quantity,ingredient_quantity_per_output,consumed_quantity,recipe_snapshot) VALUES($1,$2,$3,$4,$5,$6::numeric,$7::numeric,$8::numeric,$9::numeric,jsonb_build_object('recipe_id',$10,'output_quantity',$7::numeric,'ingredient_product_id',$5,'ingredient_quantity_base',$8::numeric))`, tc.BusinessID, tc.BranchID, saleID, sold.ProductID, item.product, sold.Quantity, outputQuantity, item.qty, required, recipeID); err != nil {
				return movementCount, err
			}
			if _, err = tx.Exec(r.Context(), `INSERT INTO inventory_movements(business_id,branch_id,product_id,quantity_delta,movement_type,reference_id,created_by) VALUES($1,$2,$3,-($4::numeric),'sale_consumption',$5,$6)`, tc.BusinessID, tc.BranchID, item.product, required, saleID, tc.UserID); err != nil {
				return movementCount, err
			}
			movementCount++
		}
	}
	return movementCount, nil
}

// normalizeCheckoutTenders keeps cash split tenders compatible while limiting M-Pesa
// to one full-value tender until mixed-rail allocation is explicitly designed.
func normalizeCheckoutTenders(in checkoutRequest, total int64) ([]checkoutTender, error) {
	t := in.Payments
	if len(t) == 0 {
		m := in.PaymentMethod
		if m == "" {
			m = "cash"
		}
		t = []checkoutTender{{Method: m, Amount: total}}
	}
	if len(t) == 0 || len(t) > 10 {
		return nil, errors.New("between_1_and_10_tenders_required")
	}
	var allocated int64
	for _, v := range t {
		if v.Method != "cash" && v.Method != "mpesa" {
			return nil, errors.New("unsupported_payment_method")
		}
		if v.Amount <= 0 || allocated > int64(^uint64(0)>>1)-v.Amount {
			return nil, errors.New("invalid_tender_amount")
		}
		allocated += v.Amount
	}
	if allocated != total {
		return nil, errors.New("tender_allocations_must_equal_sale_total")
	}
	if len(t) > 1 {
		for _, v := range t {
			if v.Method != "cash" {
				return nil, errors.New("mixed_payment_rails_not_yet_supported")
			}
		}
	}
	return t, nil
}

// normalizeCashTenders supports legacy single-cash requests and explicit split
// cash tenders. External rails remain rejected until provider verification exists.
func normalizeCashTenders(in checkoutRequest, total int64) ([]checkoutTender, error) {
	tenders := in.Payments
	if len(tenders) == 0 {
		method := in.PaymentMethod
		if method == "" {
			method = "cash"
		}
		tenders = []checkoutTender{{Method: method, Amount: total}}
	}
	if len(tenders) == 0 || len(tenders) > 10 {
		return nil, errors.New("between_1_and_10_tenders_required")
	}
	var allocated int64
	for _, t := range tenders {
		if t.Method != "cash" {
			return nil, errors.New("only_cash_tenders_enabled_until_provider_verification_is_implemented")
		}
		if t.Amount <= 0 || allocated > int64(^uint64(0)>>1)-t.Amount {
			return nil, errors.New("invalid_tender_amount")
		}
		allocated += t.Amount
	}
	if allocated != total {
		return nil, errors.New("tender_allocations_must_equal_sale_total")
	}
	return tenders, nil
}
