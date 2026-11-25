package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type createPOSOrderRequest struct {
	ClientReference string               `json:"client_reference,omitempty"`
	ServiceType     string               `json:"service_type"`
	TableID         string               `json:"table_id"`
	Label           string               `json:"label"`
	Lines           []saleQuoteInputLine `json:"lines"`
}

// createPOSOrder stores a priced order draft. It does not create a sale,
// recognize revenue, record payment, or consume stock.
func (s *server) createPOSOrder(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in createPOSOrderRequest
	if !decode(w, r, &in) {
		return
	}
	in.ServiceType = strings.TrimSpace(in.ServiceType)
	in.TableID = strings.ToLower(strings.TrimSpace(in.TableID))
	in.Label = strings.TrimSpace(in.Label)
	in.ClientReference = strings.ToLower(strings.TrimSpace(in.ClientReference))
	if in.ClientReference != "" && !validUUID(in.ClientReference) {
		jsonOut(w, 400, map[string]string{"error": "invalid_client_reference"})
		return
	}
	if in.ServiceType != "walk_in" && in.ServiceType != "table" && in.ServiceType != "tab" {
		jsonOut(w, 400, map[string]string{"error": "invalid_service_type"})
		return
	}
	if (in.ServiceType == "table") != (in.TableID != "") || (in.TableID != "" && !validUUID(in.TableID)) || len(in.Label) > 100 || len(in.Lines) > 100 {
		jsonOut(w, 400, map[string]string{"error": "invalid_order_details"})
		return
	}
	if len(in.Lines) == 0 {
		jsonOut(w, 400, map[string]string{"error": "order_requires_at_least_one_line"})
		return
	}
	if in.TableID != "" {
		var active bool
		err := tx.QueryRow(r.Context(), `SELECT is_active FROM bar_tables WHERE business_id=$1 AND branch_id=$2 AND id=$3`, tc.BusinessID, tc.BranchID, in.TableID).Scan(&active)
		if err == pgx.ErrNoRows || (err == nil && !active) {
			jsonOut(w, 422, map[string]string{"error": "table_not_available"})
			return
		}
		if err != nil {
			catalogError(w, err)
			return
		}
	}
	var requestHash string
	if in.ClientReference != "" {
		var hashErr error
		requestHash, hashErr = offlineOrderRequestHash(in)
		if hashErr != nil {
			jsonOut(w, 400, map[string]string{"error": "invalid_order_details"})
			return
		}
		lockKey := tc.BusinessID + ":" + tc.BranchID + ":offline-order:" + in.ClientReference
		if _, err := tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, lockKey); err != nil {
			catalogError(w, err)
			return
		}
		var existingID, existingNumber, existingHash, existingStatus string
		err := tx.QueryRow(r.Context(), `SELECT id::text,order_number,client_request_hash,status FROM pos_orders WHERE business_id=$1 AND branch_id=$2 AND client_reference=$3`, tc.BusinessID, tc.BranchID, in.ClientReference).Scan(&existingID, &existingNumber, &existingHash, &existingStatus)
		if err == nil {
			if existingHash != requestHash {
				jsonOut(w, 409, map[string]string{"error": "client_reference_reused_with_different_request"})
				return
			}
			jsonOut(w, 200, map[string]any{"order_id": existingID, "order_number": existingNumber, "service_type": in.ServiceType, "status": existingStatus, "idempotent_replay": true})
			return
		}
		if err != pgx.ErrNoRows {
			catalogError(w, err)
			return
		}
	}
	lines := make([]saleQuoteLine, 0, len(in.Lines))
	seen := map[string]bool{}
	for _, req := range in.Lines {
		id := strings.ToLower(strings.TrimSpace(req.ProductID))
		if !validUUID(id) || seen[id] || req.Quantity <= 0 || req.Quantity > 10000 {
			jsonOut(w, 400, map[string]string{"error": "invalid_or_duplicate_order_line"})
			return
		}
		seen[id] = true
		var sku, name string
		var price int64
		err := tx.QueryRow(r.Context(), `SELECT sku,name,sale_price_minor FROM products WHERE business_id=$1 AND is_active=true AND is_sellable=true AND id=$2`, tc.BusinessID, id).Scan(&sku, &name, &price)
		if err == pgx.ErrNoRows {
			jsonOut(w, 422, map[string]string{"error": "product_not_sellable"})
			return
		}
		if err != nil {
			catalogError(w, err)
			return
		}
		line, e := calculateSaleLine(id, sku, name, req.Quantity, price)
		if e != nil {
			jsonOut(w, 422, map[string]string{"error": "order_line_amount_invalid"})
			return
		}
		lines = append(lines, line)
	}
	totals, err := calculateSaleTotals(lines)
	if err != nil {
		jsonOut(w, 422, map[string]string{"error": "order_total_invalid"})
		return
	}
	var id string
	number := "ORD-" + tc.BranchID[:8] + "-" + newRequestID()
	err = tx.QueryRow(r.Context(), `INSERT INTO pos_orders(business_id,branch_id,order_number,service_type,table_id,label,created_by,client_reference,client_request_hash) VALUES($1,$2,$3,$4,NULLIF($5,'')::uuid,NULLIF($6,''),$7,NULLIF($8,'')::uuid,NULLIF($9,'')) RETURNING id::text`, tc.BusinessID, tc.BranchID, number, in.ServiceType, in.TableID, in.Label, tc.UserID, in.ClientReference, requestHash).Scan(&id)
	if err != nil {
		catalogError(w, err)
		return
	}
	for _, line := range lines {
		if _, err = tx.Exec(r.Context(), `INSERT INTO pos_order_lines(business_id,order_id,product_id,sku_snapshot,name_snapshot,quantity,unit_price_minor,line_total_minor) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, tc.BusinessID, id, line.ProductID, line.SKU, line.Name, line.Quantity, line.UnitPrice, line.LineTotal); err != nil {
			catalogError(w, err)
			return
		}
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary,metadata) VALUES($1,$2,'pos.order_created','pos_order',$3,'POS order created',jsonb_build_object('service_type',$4,'total_minor',$5))`, tc.BusinessID, tc.UserID, id, in.ServiceType, totals.Total); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 201, map[string]any{"order_id": id, "order_number": number, "service_type": in.ServiceType, "status": "open", "lines": lines, "totals": totals, "stock_deduction": "not_until_checkout"})
}

func offlineOrderRequestHash(in createPOSOrderRequest) (string, error) {
	canonical, err := json.Marshal(struct {
		ServiceType string               `json:"service_type"`
		TableID     string               `json:"table_id"`
		Label       string               `json:"label"`
		Lines       []saleQuoteInputLine `json:"lines"`
	}{in.ServiceType, in.TableID, in.Label, in.Lines})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func newRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "fallback"
	}
	return hex.EncodeToString(b)
}

func (s *server) listPOSOrders(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	rows, err := tx.Query(r.Context(), `SELECT o.id::text,o.order_number,o.service_type,o.status,COALESCE(o.table_id::text,''),COALESCE(o.label,''),COALESCE(o.client_reference::text,''),o.created_at,o.updated_at,COALESCE(sum(l.line_total_minor),0)::bigint,count(l.id)::int FROM pos_orders o LEFT JOIN pos_order_lines l ON l.business_id=o.business_id AND l.order_id=o.id WHERE o.business_id=$1 AND o.branch_id=$2 AND o.status IN ('open','held') GROUP BY o.id ORDER BY o.updated_at DESC LIMIT 100`, tc.BusinessID, tc.BranchID)
	if err != nil {
		catalogError(w, err)
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id, num, service, status, table, label, clientReference string
		var total int64
		var count int
		var created, updated any
		if err = rows.Scan(&id, &num, &service, &status, &table, &label, &clientReference, &created, &updated, &total, &count); err != nil {
			catalogError(w, err)
			return
		}
		out = append(out, map[string]any{"order_id": id, "order_number": num, "service_type": service, "status": status, "table_id": table, "label": label, "client_reference": clientReference, "total_minor": total, "line_count": count, "created_at": created, "updated_at": updated})
	}
	if err = rows.Err(); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"orders": out})
}

type reconcilePOSOrdersRequest struct {
	ClientReferences []string `json:"client_references"`
}

// reconcilePOSOrders performs an exact, tenant/branch-scoped lookup across all
// order statuses. It is intentionally independent of the recent open-order list.
func (s *server) reconcilePOSOrders(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in reconcilePOSOrdersRequest
	if !decode(w, r, &in) {
		return
	}
	if len(in.ClientReferences) == 0 || len(in.ClientReferences) > 100 {
		jsonOut(w, 400, map[string]string{"error": "client_references_must_contain_1_to_100_values"})
		return
	}
	refs := make([]string, 0, len(in.ClientReferences))
	seen := map[string]bool{}
	for _, raw := range in.ClientReferences {
		ref := strings.ToLower(strings.TrimSpace(raw))
		if !validUUID(ref) || seen[ref] {
			jsonOut(w, 400, map[string]string{"error": "invalid_or_duplicate_client_reference"})
			return
		}
		seen[ref] = true
		refs = append(refs, ref)
	}
	rows, err := tx.Query(r.Context(), `SELECT id::text,order_number,status,client_reference::text FROM pos_orders WHERE business_id=$1 AND branch_id=$2 AND client_reference=ANY($3::uuid[])`, tc.BusinessID, tc.BranchID, refs)
	if err != nil {
		catalogError(w, err)
		return
	}
	defer rows.Close()
	out := make([]map[string]string, 0)
	for rows.Next() {
		var id, num, status, ref string
		if err = rows.Scan(&id, &num, &status, &ref); err != nil {
			catalogError(w, err)
			return
		}
		out = append(out, map[string]string{"order_id": id, "order_number": num, "status": status, "client_reference": ref})
	}
	if err = rows.Err(); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"orders": out})
}

func (s *server) holdPOSOrder(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	id := chi.URLParam(r, "orderID")
	if !validUUID(id) {
		jsonOut(w, 400, map[string]string{"error": "invalid_order_id"})
		return
	}
	tag, err := tx.Exec(r.Context(), `UPDATE pos_orders SET status='held',updated_at=now() WHERE business_id=$1 AND branch_id=$2 AND id=$3 AND status='open'`, tc.BusinessID, tc.BranchID, id)
	if err != nil {
		catalogError(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		jsonOut(w, 409, map[string]string{"error": "order_not_open_or_not_found"})
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary) VALUES($1,$2,'pos.order_held','pos_order',$3,'POS order held')`, tc.BusinessID, tc.UserID, id); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]string{"order_id": id, "status": "held"})
}

func (s *server) retrievePOSOrder(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	id := chi.URLParam(r, "orderID")
	if !validUUID(id) {
		jsonOut(w, 400, map[string]string{"error": "invalid_order_id"})
		return
	}
	var num, service, status, table, label string
	err := tx.QueryRow(r.Context(), `SELECT order_number,service_type,status,COALESCE(table_id::text,''),COALESCE(label,'') FROM pos_orders WHERE business_id=$1 AND branch_id=$2 AND id=$3`, tc.BusinessID, tc.BranchID, id).Scan(&num, &service, &status, &table, &label)
	if err == pgx.ErrNoRows {
		jsonOut(w, 404, map[string]string{"error": "order_not_found"})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	rows, err := tx.Query(r.Context(), `SELECT product_id::text,sku_snapshot,name_snapshot,quantity,unit_price_minor,line_total_minor FROM pos_order_lines WHERE business_id=$1 AND order_id=$2 ORDER BY created_at,id`, tc.BusinessID, id)
	if err != nil {
		catalogError(w, err)
		return
	}
	defer rows.Close()
	lines := make([]map[string]any, 0)
	var total int64
	for rows.Next() {
		var p, sku, name string
		var q int
		var unit, line int64
		if err = rows.Scan(&p, &sku, &name, &q, &unit, &line); err != nil {
			catalogError(w, err)
			return
		}
		total += line
		lines = append(lines, map[string]any{"product_id": p, "sku": sku, "name": name, "quantity": q, "unit_price_minor": unit, "line_total_minor": line})
	}
	if err = rows.Err(); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"order_id": id, "order_number": num, "service_type": service, "status": status, "table_id": table, "label": label, "lines": lines, "total_minor": total})
}

type updatePOSOrderRequest struct {
	Lines []saleQuoteInputLine `json:"lines"`
}

// updatePOSOrder replaces the editable draft lines under a row lock. Prices are
// always reloaded from the active catalogue; clients cannot submit prices.
func (s *server) updatePOSOrder(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	id := chi.URLParam(r, "orderID")
	if !validUUID(id) {
		jsonOut(w, 400, map[string]string{"error": "invalid_order_id"})
		return
	}
	var in updatePOSOrderRequest
	if !decode(w, r, &in) {
		return
	}
	if len(in.Lines) == 0 || len(in.Lines) > 100 {
		jsonOut(w, 400, map[string]string{"error": "order_requires_1_to_100_lines"})
		return
	}
	var status string
	err := tx.QueryRow(r.Context(), `SELECT status FROM pos_orders WHERE business_id=$1 AND branch_id=$2 AND id=$3 FOR UPDATE`, tc.BusinessID, tc.BranchID, id).Scan(&status)
	if err == pgx.ErrNoRows {
		jsonOut(w, 404, map[string]string{"error": "order_not_found"})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	if status != "open" {
		jsonOut(w, 409, map[string]string{"error": "only_open_orders_can_be_edited"})
		return
	}
	lines := make([]saleQuoteLine, 0, len(in.Lines))
	seen := map[string]bool{}
	for _, req := range in.Lines {
		pid := strings.ToLower(strings.TrimSpace(req.ProductID))
		if !validUUID(pid) || seen[pid] || req.Quantity <= 0 || req.Quantity > 10000 {
			jsonOut(w, 400, map[string]string{"error": "invalid_or_duplicate_order_line"})
			return
		}
		seen[pid] = true
		var sku, name string
		var price int64
		err = tx.QueryRow(r.Context(), `SELECT sku,name,sale_price_minor FROM products WHERE business_id=$1 AND is_active=true AND is_sellable=true AND id=$2`, tc.BusinessID, pid).Scan(&sku, &name, &price)
		if err == pgx.ErrNoRows {
			jsonOut(w, 422, map[string]string{"error": "product_not_sellable"})
			return
		}
		if err != nil {
			catalogError(w, err)
			return
		}
		line, e := calculateSaleLine(pid, sku, name, req.Quantity, price)
		if e != nil {
			jsonOut(w, 422, map[string]string{"error": "order_line_amount_invalid"})
			return
		}
		lines = append(lines, line)
	}
	totals, err := calculateSaleTotals(lines)
	if err != nil {
		jsonOut(w, 422, map[string]string{"error": "order_total_invalid"})
		return
	}
	if _, err = tx.Exec(r.Context(), `DELETE FROM pos_order_lines WHERE business_id=$1 AND order_id=$2`, tc.BusinessID, id); err != nil {
		catalogError(w, err)
		return
	}
	for _, line := range lines {
		if _, err = tx.Exec(r.Context(), `INSERT INTO pos_order_lines(business_id,order_id,product_id,sku_snapshot,name_snapshot,quantity,unit_price_minor,line_total_minor) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, tc.BusinessID, id, line.ProductID, line.SKU, line.Name, line.Quantity, line.UnitPrice, line.LineTotal); err != nil {
			catalogError(w, err)
			return
		}
	}
	if _, err = tx.Exec(r.Context(), `UPDATE pos_orders SET updated_at=now() WHERE business_id=$1 AND id=$2`, tc.BusinessID, id); err != nil {
		catalogError(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary,metadata) VALUES($1,$2,'pos.order_updated','pos_order',$3,'POS order lines replaced',jsonb_build_object('line_count',$4,'total_minor',$5))`, tc.BusinessID, tc.UserID, id, len(lines), totals.Total); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"order_id": id, "status": "open", "lines": lines, "totals": totals})
}

func (s *server) reopenPOSOrder(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	id := chi.URLParam(r, "orderID")
	if !validUUID(id) {
		jsonOut(w, 400, map[string]string{"error": "invalid_order_id"})
		return
	}
	tag, err := tx.Exec(r.Context(), `UPDATE pos_orders SET status='open',updated_at=now() WHERE business_id=$1 AND branch_id=$2 AND id=$3 AND status='held'`, tc.BusinessID, tc.BranchID, id)
	if err != nil {
		catalogError(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		jsonOut(w, 409, map[string]string{"error": "order_not_held_or_not_found"})
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary) VALUES($1,$2,'pos.order_reopened','pos_order',$3,'POS order reopened')`, tc.BusinessID, tc.UserID, id); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]string{"order_id": id, "status": "open"})
}

func (s *server) cancelPOSOrder(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	id := chi.URLParam(r, "orderID")
	if !validUUID(id) {
		jsonOut(w, 400, map[string]string{"error": "invalid_order_id"})
		return
	}
	tag, err := tx.Exec(r.Context(), `UPDATE pos_orders SET status='cancelled',updated_at=now() WHERE business_id=$1 AND branch_id=$2 AND id=$3 AND status IN ('open','held')`, tc.BusinessID, tc.BranchID, id)
	if err != nil {
		catalogError(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		jsonOut(w, 409, map[string]string{"error": "order_not_cancellable_or_not_found"})
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary) VALUES($1,$2,'pos.order_cancelled','pos_order',$3,'POS order cancelled')`, tc.BusinessID, tc.UserID, id); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]string{"order_id": id, "status": "cancelled"})
}

// checkoutPOSOrder routes an order through the exact same transactional checkout
// implementation used by direct POS checkout. The order is locked and marked
// checked_out in that transaction.
func (s *server) checkoutPOSOrder(w http.ResponseWriter, r *http.Request) {
	id := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "orderID")))
	if !validUUID(id) {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "invalid_order_id"})
		return
	}
	var payload map[string]any
	if !decode(w, r, &payload) {
		return
	}
	payload["order_id"] = id
	body, err := json.Marshal(payload)
	if err != nil {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "invalid_checkout_request"})
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	s.checkoutSale(w, r)
}
