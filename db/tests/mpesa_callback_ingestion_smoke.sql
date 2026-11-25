-- Run as migration owner after configure-runtime-role.sh.
DO $$
DECLARE
  fn oid := 'public.ingest_mpesa_stk_callback(text,text,text,jsonb)'::regprocedure;
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_proc WHERE oid=fn AND prosecdef) THEN
    RAISE EXCEPTION 'callback ingestion function must be SECURITY DEFINER';
  END IF;
  IF NOT has_function_privilege('mekuin_app',fn,'EXECUTE') THEN
    RAISE EXCEPTION 'runtime role must be able to execute callback ingestion';
  END IF;
  IF EXISTS (SELECT 1 FROM pg_proc p, LATERAL aclexplode(COALESCE(p.proacl,acldefault('f',p.proowner))) a WHERE p.oid=fn AND a.grantee=0 AND a.privilege_type='EXECUTE') THEN
    RAISE EXCEPTION 'PUBLIC must not be able to execute callback ingestion';
  END IF;
END $$;
