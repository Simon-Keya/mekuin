-- +goose Up
-- Store non-secret per-business integration metadata only. Credentials remain deployment secrets.
CREATE TABLE business_integration_settings (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), business_id uuid NOT NULL REFERENCES businesses(id),
 provider text NOT NULL CHECK(provider IN ('mpesa','etims','card')),
 status text NOT NULL DEFAULT 'not_configured' CHECK(status IN ('not_configured','pending','live')),
 public_config jsonb NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(public_config)='object'),
 updated_by uuid, updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(business_id,provider),
 FOREIGN KEY(business_id,updated_by) REFERENCES memberships(business_id,user_id)
);
ALTER TABLE business_integration_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE business_integration_settings FORCE ROW LEVEL SECURITY;
CREATE POLICY business_integration_settings_tenant_policy ON business_integration_settings USING(business_id=nullif(current_setting('app.business_id',true),'')::uuid) WITH CHECK(business_id=nullif(current_setting('app.business_id',true),'')::uuid);
GRANT SELECT,INSERT ON business_integration_settings TO mekuin_app;
GRANT UPDATE(status,public_config,updated_by,updated_at) ON business_integration_settings TO mekuin_app;
REVOKE DELETE ON business_integration_settings FROM mekuin_app;

-- +goose Down
DROP TABLE IF EXISTS business_integration_settings;
