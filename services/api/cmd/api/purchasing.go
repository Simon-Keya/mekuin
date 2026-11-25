package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type purchaseOrderLineInput struct {
	ProductID     string `json:"product_id"`
	Quantity      string `json:"quantity"`
	UnitCostMinor int64  `json:"unit_cost_minor"`
}
type purchaseOrderInput struct {
	SupplierID string                   `json:"supplier_id"`
	Number     string                   `json:"po_number"`
	Notes      string                   `json:"notes"`
	Lines      []purchaseOrderLineInput `json:"lines"`
}
type receivePurchaseInput struct {
	Note  string `json:"note"`
	Lines []struct {
		LineID   string `json:"purchase_order_line_id"`
		Quantity string `json:"quantity"`
	} `json:"lines"`
}

// validPurchaseIdempotencyKey enforces the printable ASCII key contract used by
// receipt writes. Keeping this pure makes the boundary easy to regression-test.
func validPurchaseIdempotencyKey(key string) bool {
	key = strings.TrimSpace(key)
	if len(key) < 8 || len(key) > 128 {
		return false
	}
	return strings.IndexFunc(key, func(c rune) bool { return c < 33 || c > 126 }) < 0
}

// canonicalPurchaseQuantity makes numerically equivalent decimal strings share
// one fingerprint representation (for example, 01.5000 and 1.5).
func canonicalPurchaseQuantity(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if len(value) > 40 || !inventoryQuantityPattern.MatchString(value) {
		return "", false
	}
	parts := strings.SplitN(value, ".", 2)
	whole := strings.TrimLeft(parts[0], "0")
	if whole == "" {
		whole = "0"
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = strings.TrimRight(parts[1], "0")
	}
	if fraction == "" {
		return whole, whole != "0"
	}
	return whole + "." + fraction, whole != "0" || strings.Trim(fraction, "0") != ""
}

func purchaseOrderReceivable(status string) bool {
	return status == "ordered" || status == "partially_received"
}

func purchaseOrderCancellable(status string) bool {
	return status == "draft" || status == "ordered"
}

func canonicalReceiptFingerprint(in receivePurchaseInput) (receivePurchaseInput, string, bool) {
	in.Note = strings.TrimSpace(in.Note)
	if len(in.Note) > 300 || len(in.Lines) == 0 || len(in.Lines) > 100 {
		return in, "", false
	}
	seen := map[string]bool{}
	for i := range in.Lines {
		in.Lines[i].LineID = strings.ToLower(strings.TrimSpace(in.Lines[i].LineID))
		quantity, ok := canonicalPurchaseQuantity(in.Lines[i].Quantity)
		if !ok || !validUUID(in.Lines[i].LineID) || seen[in.Lines[i].LineID] {
			return in, "", false
		}
		in.Lines[i].Quantity = quantity
		seen[in.Lines[i].LineID] = true
	}
	sort.Slice(in.Lines, func(i, j int) bool { return in.Lines[i].LineID < in.Lines[j].LineID })
	canonical, err := json.Marshal(in)
	if err != nil {
		return in, "", false
	}
	sum := sha256.Sum256(canonical)
	return in, hex.EncodeToString(sum[:]), true
}

func (s *server) listPurchaseOrders(w http.ResponseWriter, r *http.Request) {
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
	rows, err := tx.Query(r.Context(), `SELECT id,po_number,supplier_id,status,notes,created_at,ordered_at,received_at FROM purchase_orders WHERE business_id=$1 AND branch_id=$2 AND ($3='' OR status=$3) AND ($4='' OR po_number ILIKE '%'||$4||'%') ORDER BY created_at DESC LIMIT $5 OFFSET $6`, tc.BusinessID, tc.BranchID, status, q, limit, offset)
	if err != nil {
		catalogError(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, num, supplier, state string
		var notes *string
		var created, ordered, received any
		if err = rows.Scan(&id, &num, &supplier, &state, &notes, &created, &ordered, &received); err != nil {
			catalogError(w, err)
			return
		}
		items = append(items, map[string]any{"id": id, "po_number": num, "supplier_id": supplier, "status": state, "notes": notes, "created_at": created, "ordered_at": ordered, "received_at": received})
	}
	if rows.Err() != nil {
		catalogError(w, rows.Err())
		return
	}
	jsonOut(w, 200, map[string]any{"items": items, "limit": limit, "offset": offset})
}

func (s *server) createPurchaseOrder(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in purchaseOrderInput
	if !decode(w, r, &in) {
		return
	}
	in.SupplierID = strings.ToLower(strings.TrimSpace(in.SupplierID))
	in.Number = strings.TrimSpace(in.Number)
	if !validUUID(in.SupplierID) || !requiredText(in.Number, 80) || len(in.Lines) == 0 || len(in.Lines) > 100 {
		jsonOut(w, 400, map[string]string{"error": "invalid_purchase_order"})
		return
	}
	var supplier string
	err := tx.QueryRow(r.Context(), `SELECT id FROM suppliers WHERE business_id=$1 AND id=$2 AND is_active`, tc.BusinessID, in.SupplierID).Scan(&supplier)
	if err == pgx.ErrNoRows {
		jsonOut(w, 404, map[string]string{"error": "supplier_not_found"})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	var id string
	err = tx.QueryRow(r.Context(), `INSERT INTO purchase_orders(business_id,branch_id,supplier_id,po_number,notes,created_by) VALUES($1,$2,$3,$4,NULLIF($5,''),$6) RETURNING id`, tc.BusinessID, tc.BranchID, in.SupplierID, in.Number, strings.TrimSpace(in.Notes), tc.UserID).Scan(&id)
	if err != nil {
		catalogError(w, err)
		return
	}
	seen := map[string]bool{}
	for _, line := range in.Lines {
		pid := strings.ToLower(strings.TrimSpace(line.ProductID))
		qty := strings.TrimSpace(line.Quantity)
		if !validUUID(pid) || seen[pid] || !inventoryQuantityPattern.MatchString(qty) || strings.Trim(qty, "0.") == "" || line.UnitCostMinor < 0 {
			jsonOut(w, 400, map[string]string{"error": "invalid_purchase_order_line"})
			return
		}
		seen[pid] = true
		tag, e := tx.Exec(r.Context(), `INSERT INTO purchase_order_lines(business_id,purchase_order_id,product_id,ordered_quantity,unit_cost_minor) SELECT $1,$2,id,$4::numeric,$5 FROM products WHERE business_id=$1 AND id=$3 AND is_active`, tc.BusinessID, id, pid, qty, line.UnitCostMinor)
		if e != nil {
			catalogError(w, e)
			return
		}
		if tag.RowsAffected() != 1 {
			jsonOut(w, 404, map[string]string{"error": "product_not_found"})
			return
		}
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary,metadata) VALUES($1,$2,'purchasing.order_created','purchase_order',$3,'Purchase order created',jsonb_build_object('line_count',$4))`, tc.BusinessID, tc.UserID, id, len(in.Lines))
	if err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 201, map[string]any{"id": id, "status": "draft", "po_number": in.Number})
}

func (s *server) orderPurchaseOrder(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	id := chi.URLParam(r, "orderID")
	tag, err := tx.Exec(r.Context(), `UPDATE purchase_orders SET status='ordered',ordered_at=now(),updated_at=now() WHERE business_id=$1 AND branch_id=$2 AND id=$3 AND status='draft'`, tc.BusinessID, tc.BranchID, id)
	if err != nil {
		catalogError(w, err)
		return
	}
	if tag.RowsAffected() != 1 {
		jsonOut(w, 409, map[string]string{"error": "purchase_order_not_draft_or_not_found"})
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary) VALUES($1,$2,'purchasing.order_placed','purchase_order',$3,'Purchase order marked ordered')`, tc.BusinessID, tc.UserID, id)
	if err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]string{"id": id, "status": "ordered"})
}

func (s *server) getPurchaseOrder(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	id := chi.URLParam(r, "orderID")
	var order map[string]any
	var created, ordered, received any
	var notes *string
	var supplierName string
	var status, number string
	err := tx.QueryRow(r.Context(), `SELECT po.po_number,po.status,po.notes,po.created_at,po.ordered_at,po.received_at,s.name FROM purchase_orders po JOIN suppliers s ON s.business_id=po.business_id AND s.id=po.supplier_id WHERE po.business_id=$1 AND po.branch_id=$2 AND po.id=$3`, tc.BusinessID, tc.BranchID, id).Scan(&number, &status, &notes, &created, &ordered, &received, &supplierName)
	if err == pgx.ErrNoRows {
		jsonOut(w, 404, map[string]string{"error": "purchase_order_not_found"})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	order = map[string]any{"id": id, "po_number": number, "status": status, "notes": notes, "supplier_name": supplierName, "created_at": created, "ordered_at": ordered, "received_at": received}
	rows, err := tx.Query(r.Context(), `SELECT pol.id,pol.product_id,p.name,pol.ordered_quantity::text,pol.received_quantity::text,(pol.ordered_quantity-pol.received_quantity)::text,pol.unit_cost_minor FROM purchase_order_lines pol JOIN products p ON p.business_id=pol.business_id AND p.id=pol.product_id WHERE pol.business_id=$1 AND pol.purchase_order_id=$2 ORDER BY p.name,pol.id`, tc.BusinessID, id)
	if err != nil {
		catalogError(w, err)
		return
	}
	lines := []map[string]any{}
	for rows.Next() {
		var lineID, productID, productName, qty, got, remaining string
		var cost int64
		if err = rows.Scan(&lineID, &productID, &productName, &qty, &got, &remaining, &cost); err != nil {
			rows.Close()
			catalogError(w, err)
			return
		}
		lines = append(lines, map[string]any{"id": lineID, "product_id": productID, "product_name": productName, "ordered_quantity": qty, "received_quantity": got, "remaining_quantity": remaining, "unit_cost_minor": cost})
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		catalogError(w, err)
		return
	}
	rows.Close()
	receiptsRows, err := tx.Query(r.Context(), `SELECT id,received_by,note,created_at FROM purchase_receipts WHERE business_id=$1 AND branch_id=$2 AND purchase_order_id=$3 ORDER BY created_at DESC,id`, tc.BusinessID, tc.BranchID, id)
	if err != nil {
		catalogError(w, err)
		return
	}
	receipts := []map[string]any{}
	receiptIDs := []string{}
	for receiptsRows.Next() {
		var receiptID, receivedBy string
		var note *string
		var at any
		if err = receiptsRows.Scan(&receiptID, &receivedBy, &note, &at); err != nil {
			receiptsRows.Close()
			catalogError(w, err)
			return
		}
		receiptIDs = append(receiptIDs, receiptID)
		receipts = append(receipts, map[string]any{"id": receiptID, "received_by": receivedBy, "note": note, "created_at": at, "lines": []map[string]any{}})
	}
	if err = receiptsRows.Err(); err != nil {
		receiptsRows.Close()
		catalogError(w, err)
		return
	}
	receiptsRows.Close()
	if len(receiptIDs) > 0 {
		lineRows, queryErr := tx.Query(r.Context(), `SELECT prl.receipt_id,prl.id,prl.product_id,p.name,prl.received_quantity::text,prl.unit_cost_minor,prl.movement_id FROM purchase_receipt_lines prl JOIN products p ON p.business_id=prl.business_id AND p.id=prl.product_id WHERE prl.business_id=$1 AND prl.receipt_id=ANY($2::uuid[]) ORDER BY prl.created_at,prl.id`, tc.BusinessID, receiptIDs)
		if queryErr != nil {
			catalogError(w, queryErr)
			return
		}
		byReceipt := make(map[string][]map[string]any, len(receiptIDs))
		for lineRows.Next() {
			var receiptID, lineID, productID, productName, quantity, movementID string
			var unitCost int64
			if err = lineRows.Scan(&receiptID, &lineID, &productID, &productName, &quantity, &unitCost, &movementID); err != nil {
				lineRows.Close()
				catalogError(w, err)
				return
			}
			byReceipt[receiptID] = append(byReceipt[receiptID], map[string]any{"id": lineID, "product_id": productID, "product_name": productName, "received_quantity": quantity, "unit_cost_minor": unitCost, "movement_id": movementID})
		}
		if err = lineRows.Err(); err != nil {
			lineRows.Close()
			catalogError(w, err)
			return
		}
		lineRows.Close()
		for _, receipt := range receipts {
			receipt["lines"] = byReceipt[receipt["id"].(string)]
		}
	}
	jsonOut(w, 200, map[string]any{"order": order, "lines": lines, "receipts": receipts})
}

func (s *server) cancelPurchaseOrder(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	id := chi.URLParam(r, "orderID")
	var status string
	err := tx.QueryRow(r.Context(), `SELECT status FROM purchase_orders WHERE business_id=$1 AND branch_id=$2 AND id=$3 FOR UPDATE`, tc.BusinessID, tc.BranchID, id).Scan(&status)
	if err == pgx.ErrNoRows {
		jsonOut(w, 404, map[string]string{"error": "purchase_order_not_found"})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	if !purchaseOrderCancellable(status) {
		jsonOut(w, 409, map[string]string{"error": "purchase_order_not_cancellable"})
		return
	}
	var hasReceipts bool
	if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM purchase_receipts WHERE business_id=$1 AND purchase_order_id=$2)`, tc.BusinessID, id).Scan(&hasReceipts); err != nil {
		catalogError(w, err)
		return
	}
	if hasReceipts {
		jsonOut(w, 409, map[string]string{"error": "purchase_order_has_receipts"})
		return
	}
	_, err = tx.Exec(r.Context(), `UPDATE purchase_orders SET status='cancelled',updated_at=now() WHERE business_id=$1 AND branch_id=$2 AND id=$3`, tc.BusinessID, tc.BranchID, id)
	if err != nil {
		catalogError(w, err)
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary) VALUES($1,$2,'purchasing.order_cancelled','purchase_order',$3,'Purchase order cancelled')`, tc.BusinessID, tc.UserID, id)
	if err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]string{"id": id, "status": "cancelled"})
}

func (s *server) receivePurchaseOrder(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	id := chi.URLParam(r, "orderID")
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if !validPurchaseIdempotencyKey(key) {
		jsonOut(w, 400, map[string]string{"error": "invalid_idempotency_key"})
		return
	}
	var in receivePurchaseInput
	if !decode(w, r, &in) {
		return
	}
	var fingerprint string
	var valid bool
	in, fingerprint, valid = canonicalReceiptFingerprint(in)
	if !valid {
		jsonOut(w, 400, map[string]string{"error": "invalid_receipt"})
		return
	}
	var status string
	err := tx.QueryRow(r.Context(), `SELECT status FROM purchase_orders WHERE business_id=$1 AND branch_id=$2 AND id=$3 FOR UPDATE`, tc.BusinessID, tc.BranchID, id).Scan(&status)
	if err == pgx.ErrNoRows {
		jsonOut(w, 404, map[string]string{"error": "purchase_order_not_found"})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	var existingID, existingFingerprint string
	err = tx.QueryRow(r.Context(), `SELECT id,request_fingerprint FROM purchase_receipts WHERE business_id=$1 AND purchase_order_id=$2 AND idempotency_key=$3`, tc.BusinessID, id, key).Scan(&existingID, &existingFingerprint)
	if err == nil {
		if existingFingerprint != fingerprint {
			jsonOut(w, 409, map[string]string{"error": "idempotency_key_payload_mismatch"})
			return
		}
		jsonOut(w, 200, map[string]string{"receipt_id": existingID, "purchase_order_id": id, "status": status, "replayed": true})
		return
	}
	if err != pgx.ErrNoRows {
		catalogError(w, err)
		return
	}
	if !purchaseOrderReceivable(status) {
		jsonOut(w, 409, map[string]string{"error": "purchase_order_not_receivable"})
		return
	}
	// Acquire product locks in deterministic order before checking outstanding quantities.
	productIDs := []string{}
	for _, l := range in.Lines {
		var pid string
		e := tx.QueryRow(r.Context(), `SELECT product_id FROM purchase_order_lines WHERE business_id=$1 AND purchase_order_id=$2 AND id=$3`, tc.BusinessID, id, l.LineID).Scan(&pid)
		if e == pgx.ErrNoRows {
			jsonOut(w, 404, map[string]string{"error": "purchase_order_line_not_found"})
			return
		}
		if e != nil {
			catalogError(w, e)
			return
		}
		productIDs = append(productIDs, pid)
	}
	sort.Strings(productIDs)
	for i, pid := range productIDs {
		if i > 0 && pid == productIDs[i-1] {
			continue
		}
		if e := lockInventoryBalance(r.Context(), tx, tc.BusinessID, tc.BranchID, pid); e != nil {
			catalogError(w, e)
			return
		}
	}
	var receiptID string
	err = tx.QueryRow(r.Context(), `INSERT INTO purchase_receipts(business_id,branch_id,purchase_order_id,received_by,note,idempotency_key,request_fingerprint) VALUES($1,$2,$3,$4,NULLIF($5,''),$6,$7) RETURNING id`, tc.BusinessID, tc.BranchID, id, tc.UserID, in.Note, key, fingerprint).Scan(&receiptID)
	if err != nil {
		catalogError(w, err)
		return
	}
	for _, l := range in.Lines {
		var pid, unitCost string
		var outstanding string
		err = tx.QueryRow(r.Context(), `SELECT product_id,unit_cost_minor::text,(ordered_quantity-received_quantity)::text FROM purchase_order_lines WHERE business_id=$1 AND purchase_order_id=$2 AND id=$3 FOR UPDATE`, tc.BusinessID, id, l.LineID).Scan(&pid, &unitCost, &outstanding)
		if err != nil {
			catalogError(w, err)
			return
		}
		var enough bool
		err = tx.QueryRow(r.Context(), `SELECT $1::numeric <= $2::numeric`, l.Quantity, outstanding).Scan(&enough)
		if err != nil {
			catalogError(w, err)
			return
		}
		if !enough {
			jsonOut(w, 409, map[string]string{"error": "over_receipt_quantity"})
			return
		}
		var movementID string
		err = tx.QueryRow(r.Context(), `INSERT INTO inventory_movements(business_id,branch_id,product_id,quantity_delta,movement_type,reference_id,created_by,unit_cost_minor) VALUES($1,$2,$3,$4::numeric,'purchase_receipt',$5,$6,$7::numeric) RETURNING id`, tc.BusinessID, tc.BranchID, pid, l.Quantity, receiptID, tc.UserID, unitCost).Scan(&movementID)
		if err != nil {
			catalogError(w, err)
			return
		}
		_, err = tx.Exec(r.Context(), `UPDATE purchase_order_lines SET received_quantity=received_quantity+$1::numeric WHERE business_id=$2 AND id=$3`, l.Quantity, tc.BusinessID, l.LineID)
		if err != nil {
			catalogError(w, err)
			return
		}
		_, err = tx.Exec(r.Context(), `INSERT INTO purchase_receipt_lines(business_id,receipt_id,purchase_order_line_id,product_id,received_quantity,unit_cost_minor,movement_id) VALUES($1,$2,$3,$4,$5::numeric,$6::bigint,$7)`, tc.BusinessID, receiptID, l.LineID, pid, l.Quantity, unitCost, movementID)
		if err != nil {
			catalogError(w, err)
			return
		}
	}
	var remaining bool
	err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM purchase_order_lines WHERE business_id=$1 AND purchase_order_id=$2 AND received_quantity<ordered_quantity)`, tc.BusinessID, id).Scan(&remaining)
	if err != nil {
		catalogError(w, err)
		return
	}
	next := "received"
	if remaining {
		next = "partially_received"
	}
	_, err = tx.Exec(r.Context(), `UPDATE purchase_orders SET status=$1,received_at=CASE WHEN $1='received' THEN now() ELSE received_at END,updated_at=now() WHERE business_id=$2 AND id=$3`, next, tc.BusinessID, id)
	if err != nil {
		catalogError(w, err)
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary,metadata) VALUES($1,$2,'purchasing.received','purchase_receipt',$3,'Purchase receipt posted',jsonb_build_object('purchase_order_id',$4,'line_count',$5))`, tc.BusinessID, tc.UserID, receiptID, id, len(in.Lines))
	if err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 201, map[string]any{"receipt_id": receiptID, "purchase_order_id": id, "status": next, "line_count": len(in.Lines)})
}
