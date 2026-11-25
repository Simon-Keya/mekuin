-- +goose Up
-- Phase 5C: durable operational alerts with acknowledgement/resolution history.
CREATE TABLE operational_alerts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 business_id uuid NOT NULL REFERENCES businesses(id),
 branch_id uuid NOT NULL,
 alert_type text NOT NULL CHECK(alert_type IN ('stock_out','low_stock','expense_approval','supplier_invoice_approval','cash_variance')),
 severity text NOT NULL CHECK(severity IN ('info','warning','critical')),
 source_id uuid NOT NULL,
 title text NOT NULL CHECK(length(trim(title)) BETWEEN 3 AND 180),
 description text NOT NULL CHECK(length(trim(description)) BETWEEN 3 AND 1000),
 status text NOT NULL DEFAULT 'open' CHECK(status IN ('open','acknowledged','resolved')),
 first_seen_at timestamptz NOT NULL DEFAULT now(),
 last_seen_at timestamptz NOT NULL DEFAULT now(),
 acknowledged_by uuid,
 acknowledged_at timestamptz,
 resolved_at timestamptz,
 resolution_note text,
 FOREIGN KEY(business_id,branch_id) REFERENCES branches(business_id,id),
 FOREIGN KEY(business_id,acknowledged_by) REFERENCES memberships(business_id,user_id),
 UNIQUE(business_id,branch_id,id),
 CHECK((status='acknowledged')=(acknowledged_at IS NOT NULL AND acknowledged_by IS NOT NULL)),
 CHECK((status='resolved')=(resolved_at IS NOT NULL))
);
CREATE UNIQUE INDEX operational_alerts_active_source_uq ON operational_alerts(business_id,branch_id,alert_type,source_id) WHERE status IN ('open','acknowledged');
CREATE INDEX operational_alerts_branch_status_idx ON operational_alerts(business_id,branch_id,status,severity,last_seen_at DESC);
ALTER TABLE operational_alerts ENABLE ROW LEVEL SECURITY;
ALTER TABLE operational_alerts FORCE ROW LEVEL SECURITY;
CREATE POLICY operational_alerts_tenant_policy ON operational_alerts
 USING(business_id=nullif(current_setting('app.business_id',true),'')::uuid)
 WITH CHECK(business_id=nullif(current_setting('app.business_id',true),'')::uuid);
GRANT SELECT,INSERT ON operational_alerts TO mekuin_app;
REVOKE UPDATE ON operational_alerts FROM mekuin_app;
GRANT UPDATE(status,acknowledged_by,acknowledged_at,resolved_at,resolution_note,last_seen_at,severity,title,description) ON operational_alerts TO mekuin_app;
REVOKE DELETE ON operational_alerts FROM mekuin_app;

CREATE TABLE operational_alert_events (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), business_id uuid NOT NULL, branch_id uuid NOT NULL,
 alert_id uuid NOT NULL, event_type text NOT NULL CHECK(event_type IN ('acknowledged','resolved','reopened')),
 actor_user_id uuid, note text, created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(business_id,branch_id,alert_id) REFERENCES operational_alerts(business_id,branch_id,id),
 FOREIGN KEY(business_id,actor_user_id) REFERENCES memberships(business_id,user_id)
);
CREATE INDEX operational_alert_events_alert_idx ON operational_alert_events(business_id,alert_id,created_at DESC);
ALTER TABLE operational_alert_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE operational_alert_events FORCE ROW LEVEL SECURITY;
CREATE POLICY operational_alert_events_tenant_policy ON operational_alert_events
 USING(business_id=nullif(current_setting('app.business_id',true),'')::uuid)
 WITH CHECK(business_id=nullif(current_setting('app.business_id',true),'')::uuid);
GRANT SELECT,INSERT ON operational_alert_events TO mekuin_app;
REVOKE UPDATE,DELETE ON operational_alert_events FROM mekuin_app;

INSERT INTO role_permissions(role,permission_key) VALUES
 ('manager','alerts.read'),('manager','alerts.manage'),('manager','audit.read'),
 ('stock_controller','alerts.read')
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM role_permissions WHERE role='manager' AND permission_key IN ('alerts.read','alerts.manage','audit.read');
DELETE FROM role_permissions WHERE role='stock_controller' AND permission_key='alerts.read';
DROP TABLE IF EXISTS operational_alert_events;
DROP TABLE IF EXISTS operational_alerts;
