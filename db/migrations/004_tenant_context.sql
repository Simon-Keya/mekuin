-- +goose Up
-- Business and branch context is established only after membership validation.
-- The function is intentionally narrow: it returns the caller's role only when
-- the user, business and branch are all currently active and related.
CREATE OR REPLACE FUNCTION resolve_branch_membership(p_user_id uuid, p_business_id uuid, p_branch_id uuid)
RETURNS TABLE(membership_id uuid, membership_role text)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
  SELECT m.id, m.role
  FROM memberships m
  JOIN businesses b ON b.id = m.business_id
  JOIN membership_branches mb ON mb.business_id = m.business_id AND mb.membership_id = m.id
  JOIN branches br ON br.business_id = mb.business_id AND br.id = mb.branch_id
  JOIN app_users u ON u.id = m.user_id
  WHERE m.user_id = p_user_id
    AND m.business_id = p_business_id
    AND br.id = p_branch_id
    AND m.status = 'active'
    AND b.account_status = 'active'
    AND br.is_active = true
    AND u.account_status = 'active'
  LIMIT 1
$$;
REVOKE ALL ON FUNCTION resolve_branch_membership(uuid, uuid, uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION resolve_branch_membership(uuid, uuid, uuid) TO mekuin_app;

-- +goose Down
DROP FUNCTION IF EXISTS resolve_branch_membership(uuid, uuid, uuid);
