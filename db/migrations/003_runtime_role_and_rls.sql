-- +goose Up
-- The Goose connection must use the database owner/superuser. The API must use
-- the restricted mekuin_app role configured separately in DATABASE_URL.
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'mekuin_app') THEN
    CREATE ROLE mekuin_app LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT;
  END IF;
END $$;

DO $$ BEGIN EXECUTE format('GRANT CONNECT ON DATABASE %I TO mekuin_app', current_database()); END $$;
GRANT USAGE ON SCHEMA public TO mekuin_app;
GRANT SELECT, INSERT, UPDATE ON businesses, app_users, branches, memberships, membership_branches, user_sessions, auth_login_limits TO mekuin_app;
GRANT SELECT, INSERT ON audit_logs TO mekuin_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO mekuin_app;

-- Enforce RLS even if a future deployment accidentally makes the API role a table owner.
ALTER TABLE branches FORCE ROW LEVEL SECURITY;
ALTER TABLE memberships FORCE ROW LEVEL SECURITY;
ALTER TABLE membership_branches FORCE ROW LEVEL SECURITY;
ALTER TABLE audit_logs FORCE ROW LEVEL SECURITY;

-- Membership discovery is the one deliberate cross-tenant read: it is scoped to
-- the authenticated user ID placed in a transaction-local setting by the API.
CREATE OR REPLACE FUNCTION list_current_user_workspaces()
RETURNS TABLE (
  business_id uuid,
  business_name text,
  business_status text,
  membership_role text,
  branch_id uuid,
  branch_name text
)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
  SELECT b.id, b.name, b.account_status, m.role, br.id, br.name
  FROM memberships m
  JOIN businesses b ON b.id = m.business_id
  JOIN membership_branches mb ON mb.membership_id = m.id AND mb.business_id = m.business_id
  JOIN branches br ON br.id = mb.branch_id AND br.business_id = b.id
  WHERE m.user_id = nullif(current_setting('app.user_id', true), '')::uuid
    AND m.status = 'active'
    AND br.is_active = true
  ORDER BY b.name, br.name
$$;
REVOKE ALL ON FUNCTION list_current_user_workspaces() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION list_current_user_workspaces() TO mekuin_app;

-- Default privileges for future tables created by the migration owner.
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE ON TABLES TO mekuin_app;

-- +goose Down
DROP FUNCTION IF EXISTS list_current_user_workspaces();
REVOKE ALL ON ALL TABLES IN SCHEMA public FROM mekuin_app;
ALTER TABLE audit_logs NO FORCE ROW LEVEL SECURITY;
ALTER TABLE membership_branches NO FORCE ROW LEVEL SECURITY;
ALTER TABLE memberships NO FORCE ROW LEVEL SECURITY;
ALTER TABLE branches NO FORCE ROW LEVEL SECURITY;
DROP ROLE IF EXISTS mekuin_app;
