-- Run after all migrations against PostgreSQL 17 using the application role.
DO $$
DECLARE v_count integer;
BEGIN
  SELECT count(*) INTO v_count FROM information_schema.columns
   WHERE table_schema='public' AND table_name='app_users' AND column_name IN ('mfa_secret','mfa_enabled');
  IF v_count <> 2 THEN RAISE EXCEPTION 'MFA columns missing'; END IF;
  SELECT count(*) INTO v_count FROM information_schema.tables
   WHERE table_schema='public' AND table_name IN ('auth_email_challenges','auth_mfa_challenges');
  IF v_count <> 2 THEN RAISE EXCEPTION 'authentication challenge tables missing'; END IF;
  IF to_regprocedure('lookup_auth_user(text)') IS NULL
     OR to_regprocedure('auth_set_email_verified(uuid)') IS NULL
     OR to_regprocedure('auth_set_password(uuid,text)') IS NULL
     OR to_regprocedure('auth_set_mfa(uuid,text,boolean)') IS NULL THEN
    RAISE EXCEPTION 'authentication security-definer helpers missing';
  END IF;
END $$;
