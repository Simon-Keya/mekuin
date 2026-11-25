-- +goose Up
CREATE TABLE staff_invitations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  business_id uuid NOT NULL REFERENCES businesses(id),
  invited_email text NOT NULL CHECK (length(invited_email) BETWEEN 5 AND 254),
  role text NOT NULL CHECK (role IN ('manager','cashier','bartender','stock_controller')),
  branch_ids uuid[] NOT NULL CHECK (cardinality(branch_ids) BETWEEN 1 AND 100),
  token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash)=32),
  invited_by uuid NOT NULL,
  expires_at timestamptz NOT NULL,
  accepted_at timestamptz,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (business_id, id),
  FOREIGN KEY (business_id, invited_by) REFERENCES memberships(business_id,user_id)
);
CREATE INDEX staff_invitations_pending_idx ON staff_invitations(business_id, lower(invited_email), expires_at)
  WHERE accepted_at IS NULL AND revoked_at IS NULL;
CREATE UNIQUE INDEX staff_invitations_one_pending_email_uq ON staff_invitations(business_id, lower(invited_email))
  WHERE accepted_at IS NULL AND revoked_at IS NULL;
ALTER TABLE staff_invitations ENABLE ROW LEVEL SECURITY;
ALTER TABLE staff_invitations FORCE ROW LEVEL SECURITY;
CREATE POLICY staff_invitations_tenant_policy ON staff_invitations
  USING (business_id = nullif(current_setting('app.business_id', true), '')::uuid)
  WITH CHECK (business_id = nullif(current_setting('app.business_id', true), '')::uuid);
GRANT SELECT, INSERT ON staff_invitations TO mekuin_app;
REVOKE UPDATE ON staff_invitations FROM mekuin_app;
GRANT UPDATE (revoked_at) ON staff_invitations TO mekuin_app;

-- Accepts a single-use invitation only for an authenticated, active account whose
-- email matches the invited address. SECURITY DEFINER is narrowly scoped; the
-- function owns the atomic membership + branch assignment + audit operation.
CREATE OR REPLACE FUNCTION accept_staff_invitation(p_token_hash bytea, p_user_id uuid)
RETURNS TABLE(membership_id uuid, business_id uuid, role text)
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS $$
DECLARE
  inv staff_invitations%ROWTYPE;
  account_email text;
  new_membership_id uuid;
  branch_count integer;
BEGIN
  SELECT * INTO inv FROM staff_invitations
   WHERE token_hash=p_token_hash AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at>now()
   FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION 'invitation_invalid_or_expired' USING ERRCODE='P0001'; END IF;
  SELECT lower(email) INTO account_email FROM app_users WHERE id=p_user_id AND account_status='active';
  IF account_email IS NULL OR account_email <> lower(inv.invited_email) THEN
    RAISE EXCEPTION 'invitation_email_mismatch' USING ERRCODE='P0001';
  END IF;
  UPDATE app_users SET email_verified_at=COALESCE(email_verified_at,now()) WHERE id=p_user_id;
  SELECT count(*) INTO branch_count FROM branches b
   WHERE b.business_id=inv.business_id AND b.id=ANY(inv.branch_ids) AND b.is_active=true;
  IF branch_count <> cardinality(inv.branch_ids) THEN
    RAISE EXCEPTION 'invitation_branch_unavailable' USING ERRCODE='P0001';
  END IF;
  INSERT INTO memberships(business_id,user_id,role,status)
  VALUES(inv.business_id,p_user_id,inv.role,'active') RETURNING id INTO new_membership_id;
  INSERT INTO membership_branches(business_id,membership_id,branch_id)
  SELECT inv.business_id,new_membership_id,unnest(inv.branch_ids);
  UPDATE staff_invitations SET accepted_at=now() WHERE id=inv.id;
  INSERT INTO audit_logs(business_id,actor_user_id,action,entity_type,entity_id,summary)
  VALUES(inv.business_id,p_user_id,'staff.invitation.accepted','membership',new_membership_id,'Staff invitation accepted');
  RETURN QUERY SELECT new_membership_id,inv.business_id,inv.role;
END;
$$;
REVOKE ALL ON FUNCTION accept_staff_invitation(bytea,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION accept_staff_invitation(bytea,uuid) TO mekuin_app;

CREATE OR REPLACE FUNCTION is_staff_invitation_valid(p_token_hash bytea)
RETURNS boolean
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  SELECT EXISTS(SELECT 1 FROM staff_invitations WHERE token_hash=p_token_hash AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at>now())
$$;
REVOKE ALL ON FUNCTION is_staff_invitation_valid(bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION is_staff_invitation_valid(bytea) TO mekuin_app;

-- A recipient without an account can register using the emailed invitation. The
-- invitation email, not a client-supplied email, becomes the account identity.
CREATE OR REPLACE FUNCTION register_staff_invitee(p_token_hash bytea, p_full_name text, p_password_hash text)
RETURNS TABLE(user_id uuid, email text)
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS $$
DECLARE
  inv staff_invitations%ROWTYPE;
  new_user_id uuid;
BEGIN
  SELECT * INTO inv FROM staff_invitations
   WHERE token_hash=p_token_hash AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at>now()
   FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION 'invitation_invalid_or_expired' USING ERRCODE='P0001'; END IF;
  IF length(trim(p_full_name)) NOT BETWEEN 2 AND 120 OR length(p_password_hash)<50 THEN
    RAISE EXCEPTION 'invalid_invited_account' USING ERRCODE='P0001';
  END IF;
  IF EXISTS(SELECT 1 FROM app_users WHERE lower(app_users.email)=lower(inv.invited_email)) THEN
    RAISE EXCEPTION 'invited_email_account_exists' USING ERRCODE='P0001';
  END IF;
  INSERT INTO app_users(email,full_name,password_hash,email_verified_at)
  VALUES(lower(inv.invited_email),trim(p_full_name),p_password_hash,now())
  RETURNING id INTO new_user_id;
  RETURN QUERY SELECT new_user_id,lower(inv.invited_email);
END;
$$;
REVOKE ALL ON FUNCTION register_staff_invitee(bytea,text,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION register_staff_invitee(bytea,text,text) TO mekuin_app;

-- +goose Down
REVOKE EXECUTE ON FUNCTION accept_staff_invitation(bytea,uuid) FROM mekuin_app;
REVOKE EXECUTE ON FUNCTION is_staff_invitation_valid(bytea) FROM mekuin_app;
DROP FUNCTION IF EXISTS is_staff_invitation_valid(bytea);
REVOKE EXECUTE ON FUNCTION register_staff_invitee(bytea,text,text) FROM mekuin_app;
DROP FUNCTION IF EXISTS register_staff_invitee(bytea,text,text);
DROP FUNCTION IF EXISTS accept_staff_invitation(bytea,uuid);
DROP TABLE IF EXISTS staff_invitations;
