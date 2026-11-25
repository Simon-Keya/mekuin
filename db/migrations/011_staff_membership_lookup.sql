-- +goose Up
-- Narrow SECURITY DEFINER lookup: exposes only the UUID for an exact email
-- match. It does not expose profile, password, account status or other tenants.
CREATE OR REPLACE FUNCTION lookup_staff_user(p_email text)
RETURNS TABLE(user_id uuid)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  SELECT u.id FROM app_users u WHERE lower(u.email)=lower(trim(p_email)) AND u.account_status='active' LIMIT 1
$$;
REVOKE ALL ON FUNCTION lookup_staff_user(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION lookup_staff_user(text) TO mekuin_app;

-- +goose Down
REVOKE EXECUTE ON FUNCTION lookup_staff_user(text) FROM mekuin_app;
DROP FUNCTION IF EXISTS lookup_staff_user(text);
