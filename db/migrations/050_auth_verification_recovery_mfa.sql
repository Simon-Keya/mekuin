-- +goose Up
ALTER TABLE app_users ADD COLUMN IF NOT EXISTS mfa_secret text;
ALTER TABLE app_users ADD COLUMN IF NOT EXISTS mfa_enabled boolean NOT NULL DEFAULT false;
CREATE TABLE IF NOT EXISTS auth_email_challenges (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
  purpose text NOT NULL CHECK (purpose IN ('verify_email','password_reset')),
  code_hash bytea NOT NULL,
  expires_at timestamptz NOT NULL,
  attempts integer NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 10),
  consumed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS auth_email_challenges_user_purpose_idx ON auth_email_challenges(user_id,purpose,created_at DESC);
CREATE TABLE IF NOT EXISTS auth_mfa_challenges (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
  code_hash bytea NOT NULL,
  expires_at timestamptz NOT NULL,
  attempts integer NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 10),
  consumed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS auth_mfa_challenges_user_idx ON auth_mfa_challenges(user_id,created_at DESC);
REVOKE ALL ON auth_email_challenges,auth_mfa_challenges FROM PUBLIC;
-- Narrow SECURITY DEFINER helpers preserve identity-table RLS boundaries.
DROP FUNCTION IF EXISTS lookup_auth_user(text);
CREATE FUNCTION lookup_auth_user(p_email text)
RETURNS TABLE(user_id uuid,password_hash text,account_status text,email_verified_at timestamptz,mfa_enabled boolean)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=public,pg_temp AS $$
 SELECT u.id,u.password_hash,u.account_status,u.email_verified_at,u.mfa_enabled
 FROM app_users u WHERE lower(u.email)=lower(p_email) LIMIT 1
$$;
REVOKE ALL ON FUNCTION lookup_auth_user(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION lookup_auth_user(text) TO mekuin_app;
CREATE OR REPLACE FUNCTION auth_set_email_verified(p_challenge_id uuid)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=public,pg_temp AS $$
DECLARE v_user_id uuid;
BEGIN
 SELECT user_id INTO v_user_id FROM auth_email_challenges WHERE id=p_challenge_id AND purpose='verify_email' AND consumed_at IS NULL AND expires_at>now() AND attempts<10 FOR UPDATE;
 IF v_user_id IS NULL THEN RAISE EXCEPTION 'invalid_or_expired_verification_code'; END IF;
 UPDATE auth_email_challenges SET consumed_at=now() WHERE id=p_challenge_id;
 UPDATE app_users SET email_verified_at=coalesce(email_verified_at,now()),updated_at=now() WHERE id=v_user_id;
END $$;
CREATE OR REPLACE FUNCTION auth_set_password(p_challenge_id uuid,p_password_hash text)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=public,pg_temp AS $$
DECLARE v_user_id uuid;
BEGIN
 SELECT user_id INTO v_user_id FROM auth_email_challenges WHERE id=p_challenge_id AND purpose='password_reset' AND consumed_at IS NULL AND expires_at>now() AND attempts<10 FOR UPDATE;
 IF v_user_id IS NULL OR length(p_password_hash)<50 THEN RAISE EXCEPTION 'invalid_or_expired_reset_code'; END IF;
 UPDATE app_users SET password_hash=p_password_hash,updated_at=now() WHERE id=v_user_id AND email_verified_at IS NOT NULL AND account_status='active';
 UPDATE auth_email_challenges SET consumed_at=now() WHERE id=p_challenge_id;
 UPDATE user_sessions SET revoked_at=now() WHERE user_id=v_user_id AND revoked_at IS NULL;
END $$;
CREATE OR REPLACE FUNCTION auth_set_mfa(p_user_id uuid,p_secret text,p_enabled boolean)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=public,pg_temp AS $$
BEGIN
 IF nullif(current_setting('app.user_id',true),'')::uuid IS DISTINCT FROM p_user_id THEN RAISE EXCEPTION 'identity_context_mismatch'; END IF;
 UPDATE app_users SET mfa_secret=p_secret,mfa_enabled=p_enabled,updated_at=now() WHERE id=p_user_id;
END $$;
CREATE OR REPLACE FUNCTION lookup_auth_user_by_id(p_user_id uuid)
RETURNS TABLE(email text,mfa_secret text,mfa_enabled boolean,account_status text,email_verified_at timestamptz)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=public,pg_temp AS $$
 SELECT u.email,u.mfa_secret,u.mfa_enabled,u.account_status,u.email_verified_at FROM app_users u
 WHERE u.id=p_user_id AND nullif(current_setting('app.user_id',true),'')::uuid=p_user_id LIMIT 1
$$;
CREATE OR REPLACE FUNCTION lookup_mfa_challenge(p_challenge_id uuid)
RETURNS TABLE(challenge_id uuid,user_id uuid,mfa_secret text,account_status text,email_verified_at timestamptz,mfa_enabled boolean)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=public,pg_temp AS $$
 SELECT c.id,u.id,u.mfa_secret,u.account_status,u.email_verified_at,u.mfa_enabled
 FROM auth_mfa_challenges c JOIN app_users u ON u.id=c.user_id
 WHERE c.id=p_challenge_id AND c.consumed_at IS NULL AND c.expires_at>now() AND c.attempts<10 LIMIT 1
$$;
REVOKE ALL ON FUNCTION lookup_auth_user_by_id(uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION lookup_mfa_challenge(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION lookup_auth_user_by_id(uuid),lookup_mfa_challenge(uuid) TO mekuin_app;
REVOKE ALL ON FUNCTION auth_set_email_verified(uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION auth_set_password(uuid,text) FROM PUBLIC;
REVOKE ALL ON FUNCTION auth_set_mfa(uuid,text,boolean) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION auth_set_email_verified(uuid),auth_set_password(uuid,text),auth_set_mfa(uuid,text,boolean) TO mekuin_app;
GRANT SELECT,INSERT,UPDATE ON auth_email_challenges,auth_mfa_challenges TO mekuin_app;

-- +goose Down
DROP FUNCTION IF EXISTS lookup_mfa_challenge(uuid);
DROP FUNCTION IF EXISTS lookup_auth_user_by_id(uuid);
DROP FUNCTION IF EXISTS auth_set_mfa(uuid,text,boolean);
DROP FUNCTION IF EXISTS auth_set_password(uuid,text);
DROP FUNCTION IF EXISTS auth_set_email_verified(uuid);
DROP FUNCTION IF EXISTS lookup_auth_user(text);
DROP TABLE IF EXISTS auth_mfa_challenges;
DROP TABLE IF EXISTS auth_email_challenges;
ALTER TABLE app_users DROP COLUMN IF EXISTS mfa_enabled;
ALTER TABLE app_users DROP COLUMN IF EXISTS mfa_secret;
CREATE FUNCTION lookup_auth_user(p_email text)
RETURNS TABLE(user_id uuid,password_hash text,account_status text)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=public,pg_temp AS $$
 SELECT u.id,u.password_hash,u.account_status FROM app_users u WHERE lower(u.email)=lower(p_email) LIMIT 1
$$;
REVOKE ALL ON FUNCTION lookup_auth_user(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION lookup_auth_user(text) TO mekuin_app;
