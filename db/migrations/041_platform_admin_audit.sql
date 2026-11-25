-- +goose Up
CREATE TABLE platform_audit_logs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  actor_user_id uuid REFERENCES app_users(id),
  action text NOT NULL,
  entity_type text NOT NULL,
  entity_id uuid NOT NULL,
  reason text NOT NULL,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX platform_audit_logs_entity_time_idx ON platform_audit_logs(entity_type, entity_id, created_at DESC);
CREATE INDEX platform_audit_logs_time_idx ON platform_audit_logs(created_at DESC);
REVOKE UPDATE, DELETE ON platform_audit_logs FROM PUBLIC;

GRANT SELECT, INSERT ON platform_audit_logs TO mekuin_app;

-- +goose Down
DROP TABLE IF EXISTS platform_audit_logs;
