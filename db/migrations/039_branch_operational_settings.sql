-- +goose Up
CREATE TABLE branch_operational_settings (
 business_id uuid NOT NULL,
 branch_id uuid NOT NULL,
 cash_variance_alert_threshold_minor bigint NOT NULL DEFAULT 0 CHECK(cash_variance_alert_threshold_minor >= 0),
 low_stock_alerts_enabled boolean NOT NULL DEFAULT true,
 stock_out_alerts_enabled boolean NOT NULL DEFAULT true,
 expense_approval_alerts_enabled boolean NOT NULL DEFAULT true,
 supplier_invoice_alerts_enabled boolean NOT NULL DEFAULT true,
 cash_variance_alerts_enabled boolean NOT NULL DEFAULT true,
 updated_by uuid,
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(business_id,branch_id),
 FOREIGN KEY(business_id,branch_id) REFERENCES branches(business_id,id),
 FOREIGN KEY(business_id,updated_by) REFERENCES memberships(business_id,user_id)
);
ALTER TABLE branch_operational_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE branch_operational_settings FORCE ROW LEVEL SECURITY;
CREATE POLICY branch_operational_settings_tenant_policy ON branch_operational_settings
 USING(business_id=nullif(current_setting('app.business_id',true),'')::uuid)
 WITH CHECK(business_id=nullif(current_setting('app.business_id',true),'')::uuid);
GRANT SELECT,INSERT,UPDATE ON branch_operational_settings TO mekuin_app;
REVOKE DELETE,TRUNCATE ON branch_operational_settings FROM mekuin_app;
INSERT INTO role_permissions(role,permission_key) VALUES ('manager','settings.read'),('manager','settings.manage') ON CONFLICT DO NOTHING;
-- +goose Down
DELETE FROM role_permissions WHERE role='manager' AND permission_key IN ('settings.read','settings.manage');
DROP TABLE IF EXISTS branch_operational_settings;
