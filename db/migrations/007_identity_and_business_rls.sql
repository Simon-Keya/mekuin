-- +goose Up
-- Complete RLS coverage for platform identity, business records and sessions.
-- Authentication lookups use narrowly-scoped SECURITY DEFINER functions owned
-- by the migration owner; the runtime role cannot read other users or sessions.

ALTER TABLE businesses ENABLE ROW LEVEL SECURITY;
ALTER TABLE businesses FORCE ROW LEVEL SECURITY;
CREATE POLICY businesses_tenant_read ON businesses
  FOR SELECT USING (id = nullif(current_setting('app.business_id', true), '')::uuid);
CREATE POLICY businesses_registration_read ON businesses
  FOR SELECT USING (current_setting('app.registration', true) = 'true');
CREATE POLICY businesses_registration_insert ON businesses
  FOR INSERT WITH CHECK (current_setting('app.registration', true) = 'true');

ALTER TABLE app_users ENABLE ROW LEVEL SECURITY;
ALTER TABLE app_users FORCE ROW LEVEL SECURITY;
CREATE POLICY app_users_self_read ON app_users
  FOR SELECT USING (id = nullif(current_setting('app.user_id', true), '')::uuid);
CREATE POLICY app_users_registration_read ON app_users
  FOR SELECT USING (current_setting('app.registration', true) = 'true');
CREATE POLICY app_users_registration_insert ON app_users
  FOR INSERT WITH CHECK (current_setting('app.registration', true) = 'true');

ALTER TABLE user_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_sessions FORCE ROW LEVEL SECURITY;
CREATE POLICY user_sessions_self_read ON user_sessions
  FOR SELECT USING (user_id = nullif(current_setting('app.user_id', true), '')::uuid);
CREATE POLICY user_sessions_self_insert ON user_sessions
  FOR INSERT WITH CHECK (user_id = nullif(current_setting('app.user_id', true), '')::uuid);
CREATE POLICY user_sessions_self_update ON user_sessions
  FOR UPDATE USING (user_id = nullif(current_setting('app.user_id', true), '')::uuid)
  WITH CHECK (user_id = nullif(current_setting('app.user_id', true), '')::uuid);

GRANT DELETE ON auth_login_limits TO mekuin_app;
REVOKE UPDATE ON app_users FROM mekuin_app;
REVOKE UPDATE ON user_sessions FROM mekuin_app;
GRANT UPDATE (selected_business_id, selected_branch_id, revoked_at) ON user_sessions TO mekuin_app;

CREATE OR REPLACE FUNCTION lookup_auth_user(p_email text)
RETURNS TABLE(user_id uuid, password_hash text, account_status text)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  SELECT u.id, u.password_hash, u.account_status
  FROM app_users u WHERE lower(u.email) = lower(p_email) LIMIT 1
$$;
REVOKE ALL ON FUNCTION lookup_auth_user(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION lookup_auth_user(text) TO mekuin_app;

CREATE OR REPLACE FUNCTION lookup_session(p_token_hash bytea)
RETURNS TABLE(user_id uuid, account_status text, session_id uuid)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  SELECT ss.user_id, u.account_status, ss.id
  FROM user_sessions ss JOIN app_users u ON u.id = ss.user_id
  WHERE ss.token_hash = p_token_hash AND ss.revoked_at IS NULL AND ss.expires_at > now()
  LIMIT 1
$$;
REVOKE ALL ON FUNCTION lookup_session(bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION lookup_session(bytea) TO mekuin_app;

-- +goose Down
DROP FUNCTION IF EXISTS lookup_session(bytea);
DROP FUNCTION IF EXISTS lookup_auth_user(text);
DROP POLICY IF EXISTS user_sessions_self_update ON user_sessions;
DROP POLICY IF EXISTS user_sessions_self_insert ON user_sessions;
DROP POLICY IF EXISTS user_sessions_self_read ON user_sessions;
ALTER TABLE user_sessions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE user_sessions DISABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS app_users_registration_read ON app_users;
DROP POLICY IF EXISTS app_users_registration_insert ON app_users;
DROP POLICY IF EXISTS app_users_self_read ON app_users;
ALTER TABLE app_users NO FORCE ROW LEVEL SECURITY;
ALTER TABLE app_users DISABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS businesses_registration_read ON businesses;
DROP POLICY IF EXISTS businesses_registration_insert ON businesses;
DROP POLICY IF EXISTS businesses_tenant_read ON businesses;
ALTER TABLE businesses NO FORCE ROW LEVEL SECURITY;
ALTER TABLE businesses DISABLE ROW LEVEL SECURITY;
REVOKE DELETE ON auth_login_limits FROM mekuin_app;
GRANT UPDATE ON app_users TO mekuin_app;
GRANT UPDATE ON user_sessions TO mekuin_app;
