package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// refreshOperationalAlerts materializes current conditions. Source records remain
// authoritative; this table supplies durable acknowledgement and resolution.
func refreshOperationalAlerts(r *http.Request, tx pgx.Tx, tc tenantContext) error {
	ctx := r.Context()
	statements := []string{
		`INSERT INTO operational_alerts(business_id,branch_id,alert_type,severity,source_id,title,description)
   SELECT p.business_id,$2,'stock_out','critical',p.id,'Out of stock: '||p.name,'On-hand quantity is zero or below the configured reorder level.'
   FROM products p JOIN inventory_movements m ON m.business_id=p.business_id AND m.branch_id=$2 AND m.product_id=p.id
   WHERE p.business_id=$1 AND p.is_active AND COALESCE((SELECT stock_out_alerts_enabled FROM branch_operational_settings WHERE business_id=$1 AND branch_id=$2),true) GROUP BY p.business_id,p.id,p.name,p.reorder_level
   HAVING COALESCE(sum(m.quantity_delta),0)<=0
   ON CONFLICT (business_id,branch_id,alert_type,source_id) WHERE status IN ('open','acknowledged')
   DO UPDATE SET last_seen_at=now(),title=EXCLUDED.title,description=EXCLUDED.description`,
		`INSERT INTO operational_alerts(business_id,branch_id,alert_type,severity,source_id,title,description)
   SELECT p.business_id,$2,'low_stock','warning',p.id,'Low stock: '||p.name,'On-hand quantity is at or below the configured reorder level.'
   FROM products p JOIN inventory_movements m ON m.business_id=p.business_id AND m.branch_id=$2 AND m.product_id=p.id
   WHERE p.business_id=$1 AND p.is_active AND p.reorder_level>0 AND COALESCE((SELECT low_stock_alerts_enabled FROM branch_operational_settings WHERE business_id=$1 AND branch_id=$2),true) GROUP BY p.business_id,p.id,p.name,p.reorder_level
   HAVING COALESCE(sum(m.quantity_delta),0)>0 AND COALESCE(sum(m.quantity_delta),0)<=p.reorder_level
   ON CONFLICT (business_id,branch_id,alert_type,source_id) WHERE status IN ('open','acknowledged')
   DO UPDATE SET last_seen_at=now(),title=EXCLUDED.title,description=EXCLUDED.description`,
		`INSERT INTO operational_alerts(business_id,branch_id,alert_type,severity,source_id,title,description)
   SELECT e.business_id,e.branch_id,'expense_approval','warning',e.id,'Expense awaiting approval','Expense: '||e.description||' · '||e.currency||' '||e.amount_minor::text
   FROM expenses e WHERE e.business_id=$1 AND e.branch_id=$2 AND e.status='submitted' AND COALESCE((SELECT expense_approval_alerts_enabled FROM branch_operational_settings WHERE business_id=$1 AND branch_id=$2),true)
   ON CONFLICT (business_id,branch_id,alert_type,source_id) WHERE status IN ('open','acknowledged')
   DO UPDATE SET last_seen_at=now(),title=EXCLUDED.title,description=EXCLUDED.description`,
		`INSERT INTO operational_alerts(business_id,branch_id,alert_type,severity,source_id,title,description)
   SELECT i.business_id,i.branch_id,'supplier_invoice_approval','warning',i.id,'Supplier invoice awaiting review','Invoice '||i.invoice_number||' is awaiting an approval decision.'
   FROM supplier_invoices i JOIN LATERAL (SELECT event_type FROM supplier_invoice_review_events e WHERE e.business_id=i.business_id AND e.invoice_id=i.id ORDER BY event_no DESC LIMIT 1) latest ON latest.event_type='submitted'
   WHERE i.business_id=$1 AND i.branch_id=$2 AND i.status NOT IN ('paid','void') AND COALESCE((SELECT supplier_invoice_alerts_enabled FROM branch_operational_settings WHERE business_id=$1 AND branch_id=$2),true)
   ON CONFLICT (business_id,branch_id,alert_type,source_id) WHERE status IN ('open','acknowledged')
   DO UPDATE SET last_seen_at=now(),title=EXCLUDED.title,description=EXCLUDED.description`,
		`INSERT INTO operational_alerts(business_id,branch_id,alert_type,severity,source_id,title,description)
   SELECT c.business_id,c.branch_id,'cash_variance',CASE WHEN COALESCE((SELECT cash_variance_alert_threshold_minor FROM branch_operational_settings WHERE business_id=$1 AND branch_id=$2),0)>0 AND abs(c.variance_minor)>=COALESCE((SELECT cash_variance_alert_threshold_minor FROM branch_operational_settings WHERE business_id=$1 AND branch_id=$2),0) THEN 'critical' ELSE 'warning' END,c.id,'Cash shift variance detected','Closed cash shift variance: '||c.variance_minor::text||' minor units.'
   FROM cash_shifts c WHERE c.business_id=$1 AND c.branch_id=$2 AND c.status='closed' AND c.variance_minor<>0 AND COALESCE((SELECT cash_variance_alerts_enabled FROM branch_operational_settings WHERE business_id=$1 AND branch_id=$2),true)
   ON CONFLICT (business_id,branch_id,alert_type,source_id) WHERE status IN ('open','acknowledged')
   DO UPDATE SET last_seen_at=now(),severity=EXCLUDED.severity,title=EXCLUDED.title,description=EXCLUDED.description`,
	}
	for _, q := range statements {
		if _, err := tx.Exec(ctx, q, tc.BusinessID, tc.BranchID); err != nil {
			return err
		}
	}
	// Resolve only alerts whose source condition is no longer present. Acknowledged
	// alerts are also resolved when the underlying condition clears.
	_, err := tx.Exec(ctx, `WITH resolved AS (UPDATE operational_alerts a SET status='resolved',resolved_at=now(),acknowledged_by=NULL,acknowledged_at=NULL,resolution_note='Automatically resolved because the source condition cleared.'
  WHERE a.business_id=$1 AND a.branch_id=$2 AND a.status IN ('open','acknowledged') AND NOT (
   (a.alert_type='stock_out' AND EXISTS(SELECT 1 FROM products p JOIN inventory_movements m ON m.business_id=p.business_id AND m.branch_id=a.branch_id AND m.product_id=p.id WHERE p.business_id=a.business_id AND p.id=a.source_id AND p.is_active GROUP BY p.id HAVING COALESCE(sum(m.quantity_delta),0)<=0)) OR
   (a.alert_type='low_stock' AND EXISTS(SELECT 1 FROM products p JOIN inventory_movements m ON m.business_id=p.business_id AND m.branch_id=a.branch_id AND m.product_id=p.id WHERE p.business_id=a.business_id AND p.id=a.source_id AND p.is_active AND p.reorder_level>0 GROUP BY p.id HAVING COALESCE(sum(m.quantity_delta),0)>0 AND COALESCE(sum(m.quantity_delta),0)<=p.reorder_level)) OR
   (a.alert_type='expense_approval' AND EXISTS(SELECT 1 FROM expenses e WHERE e.business_id=a.business_id AND e.branch_id=a.branch_id AND e.id=a.source_id AND e.status='submitted')) OR
   (a.alert_type='supplier_invoice_approval' AND EXISTS(SELECT 1 FROM supplier_invoices i JOIN LATERAL (SELECT event_type FROM supplier_invoice_review_events e WHERE e.business_id=i.business_id AND e.invoice_id=i.id ORDER BY event_no DESC LIMIT 1) latest ON latest.event_type='submitted' WHERE i.business_id=a.business_id AND i.branch_id=a.branch_id AND i.id=a.source_id AND i.status NOT IN ('paid','void'))) OR
   (a.alert_type='cash_variance' AND EXISTS(SELECT 1 FROM cash_shifts c WHERE c.business_id=a.business_id AND c.branch_id=a.branch_id AND c.id=a.source_id AND c.status='closed' AND c.variance_minor<>0))
  ) RETURNING id,business_id,branch_id), event_rows AS (INSERT INTO operational_alert_events(business_id,branch_id,alert_id,event_type,actor_user_id,note) SELECT business_id,branch_id,id,'resolved',NULL,'Automatically resolved because the source condition cleared.' FROM resolved RETURNING business_id,alert_id) INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary,metadata) SELECT business_id,NULL,'alerts.auto_resolved','operational_alert',alert_id,'Operational alert automatically resolved','{}'::jsonb FROM event_rows`, tc.BusinessID, tc.BranchID)
	return err
}

func (s *server) listOperationalAlerts(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	if err := refreshOperationalAlerts(r, tx, tc); err != nil {
		catalogError(w, err)
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status != "" && status != "all" && status != "open" && status != "acknowledged" && status != "resolved" {
		jsonOut(w, 400, map[string]string{"error": "invalid_status"})
		return
	}
	rows, err := tx.Query(r.Context(), `SELECT id::text,alert_type,severity,source_id::text,title,description,status,first_seen_at,last_seen_at,acknowledged_by::text,acknowledged_at,resolved_at,resolution_note FROM operational_alerts WHERE business_id=$1 AND branch_id=$2 AND ($3='' OR $3='all' OR status=$3) ORDER BY CASE severity WHEN 'critical' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END,last_seen_at DESC LIMIT 250`, tc.BusinessID, tc.BranchID, status)
	if err != nil {
		catalogError(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, typ, severity, source, title, description, state string
		var first, last, ackAt, resAt any
		var ackBy, note *string
		if err = rows.Scan(&id, &typ, &severity, &source, &title, &description, &state, &first, &last, &ackBy, &ackAt, &resAt, &note); err != nil {
			catalogError(w, err)
			return
		}
		items = append(items, map[string]any{"id": id, "alert_type": typ, "severity": severity, "source_id": source, "title": title, "description": description, "status": state, "first_seen_at": first, "last_seen_at": last, "acknowledged_by": ackBy, "acknowledged_at": ackAt, "resolved_at": resAt, "resolution_note": note})
	}
	if err = rows.Err(); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"items": items})
}

func (s *server) updateOperationalAlert(w http.ResponseWriter, r *http.Request) {
	tx, tc, ok := catalogTx(r)
	if !ok {
		jsonOut(w, 500, map[string]string{"error": "tenant_context_missing"})
		return
	}
	var in struct {
		Action string `json:"action"`
		Note   string `json:"note"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Action = strings.TrimSpace(in.Action)
	in.Note = strings.TrimSpace(in.Note)
	if (in.Action != "acknowledge" && in.Action != "resolve") || len(in.Note) > 1000 {
		jsonOut(w, 400, map[string]string{"error": "invalid_alert_action"})
		return
	}
	id := chi.URLParam(r, "alertID")
	var previous string
	err := tx.QueryRow(r.Context(), `SELECT status FROM operational_alerts WHERE business_id=$1 AND branch_id=$2 AND id=$3 FOR UPDATE`, tc.BusinessID, tc.BranchID, id).Scan(&previous)
	if err == pgx.ErrNoRows {
		jsonOut(w, 404, map[string]string{"error": "alert_not_found"})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	if previous == "resolved" {
		jsonOut(w, 409, map[string]string{"error": "alert_already_resolved"})
		return
	}
	event := in.Action + "d"
	if in.Action == "acknowledge" {
		event = "acknowledged"
	}
	if in.Action == "acknowledge" {
		_, err = tx.Exec(r.Context(), `UPDATE operational_alerts SET status='acknowledged',acknowledged_by=$4,acknowledged_at=now() WHERE business_id=$1 AND branch_id=$2 AND id=$3`, tc.BusinessID, tc.BranchID, id, tc.UserID)
	} else {
		_, err = tx.Exec(r.Context(), `UPDATE operational_alerts SET status='resolved',resolved_at=now(),acknowledged_by=NULL,acknowledged_at=NULL,resolution_note=NULLIF($4,'') WHERE business_id=$1 AND branch_id=$2 AND id=$3`, tc.BusinessID, tc.BranchID, id, in.Note)
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO operational_alert_events(business_id,branch_id,alert_id,event_type,actor_user_id,note) VALUES($1,$2,$3,$4,$5,NULLIF($6,''))`, tc.BusinessID, tc.BranchID, id, event, tc.UserID, in.Note); err != nil {
		catalogError(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary,metadata) VALUES($1,$2,$3,'operational_alert',$4,'Operational alert updated',jsonb_build_object('action',$5,'note',$6))`, tc.BusinessID, tc.UserID, "alerts."+event, id, in.Action, in.Note); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"id": id, "status": map[bool]string{true: "acknowledged", false: "resolved"}[in.Action == "acknowledge"]})
}

func (s *server) listAuditLedger(w http.ResponseWriter, r *http.Request) {
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
	action := strings.TrimSpace(r.URL.Query().Get("action"))
	entity := strings.TrimSpace(r.URL.Query().Get("entity_type"))
	if len(action) > 100 || len(entity) > 80 {
		jsonOut(w, 400, map[string]string{"error": "invalid_audit_filter"})
		return
	}
	rows, err := tx.Query(r.Context(), `SELECT a.id::text,a.actor_user_id::text,COALESCE(u.full_name,'System'),a.action,a.entity_type,a.entity_id::text,a.summary,a.metadata,a.created_at FROM audit_logs a LEFT JOIN app_users u ON u.id=a.actor_user_id WHERE a.business_id=$1 AND ($2='' OR a.action=$2) AND ($3='' OR a.entity_type=$3) AND ($4='' OR a.summary ILIKE '%'||$4||'%' OR a.action ILIKE '%'||$4||'%' OR a.entity_type ILIKE '%'||$4||'%') ORDER BY a.created_at DESC,a.id DESC LIMIT $5 OFFSET $6`, tc.BusinessID, action, entity, q, limit, offset)
	if err != nil {
		catalogError(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, name, act, typ, summary string
		var actor, entityID *string
		var metadata json.RawMessage
		var created any
		if err = rows.Scan(&id, &actor, &name, &act, &typ, &entityID, &summary, &metadata, &created); err != nil {
			catalogError(w, err)
			return
		}
		items = append(items, map[string]any{"id": id, "actor_user_id": actor, "actor_name": name, "action": act, "entity_type": typ, "entity_id": entityID, "summary": summary, "metadata": metadata, "created_at": created})
	}
	if err = rows.Err(); err != nil {
		catalogError(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"items": items, "limit": limit, "offset": offset})
}
