-- +goose Up
-- Keep platform audit history append-only for the application runtime role.
REVOKE UPDATE, DELETE, TRUNCATE, REFERENCES, TRIGGER ON platform_audit_logs FROM PUBLIC;
REVOKE UPDATE, DELETE, TRUNCATE, REFERENCES, TRIGGER ON platform_audit_logs FROM mekuin_app;
GRANT SELECT, INSERT ON platform_audit_logs TO mekuin_app;

-- The platform endpoints perform narrowly-scoped lifecycle changes. Do not
-- grant the runtime role any additional broad platform-table privileges here.

-- +goose Down
-- The original 041 migration already grants SELECT/INSERT only to mekuin_app.
-- No privilege expansion is performed by this migration, so rollback is a no-op.
