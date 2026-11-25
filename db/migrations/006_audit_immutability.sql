-- +goose Up
-- Audit records are append-only for the application role and ordinary SQL paths.
CREATE OR REPLACE FUNCTION reject_audit_log_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  RAISE EXCEPTION 'audit_logs are append-only';
END;
$$;

CREATE TRIGGER audit_logs_no_update
BEFORE UPDATE OR DELETE ON audit_logs
FOR EACH ROW EXECUTE FUNCTION reject_audit_log_mutation();
REVOKE UPDATE, DELETE, TRUNCATE ON audit_logs FROM mekuin_app;

-- +goose Down
DROP TRIGGER IF EXISTS audit_logs_no_update ON audit_logs;
DROP FUNCTION IF EXISTS reject_audit_log_mutation();
GRANT UPDATE, DELETE ON audit_logs TO mekuin_app;
