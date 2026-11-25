-- Run as migration owner after runtime-role provisioning.
DO $$
DECLARE
  fn text;
BEGIN
  IF to_regclass('public.mpesa_unmatched_callback_events') IS NULL THEN
    RAISE EXCEPTION 'unmatched callback inbox is missing';
  END IF;
  IF has_table_privilege('mekuin_app','public.mpesa_unmatched_callback_events','SELECT')
    OR has_table_privilege('mekuin_app','public.mpesa_unmatched_callback_events','INSERT')
    OR has_table_privilege('mekuin_app','public.mpesa_unmatched_callback_events','UPDATE') THEN
    RAISE EXCEPTION 'runtime role must not access the global unmatched callback inbox directly';
  END IF;
  IF NOT has_function_privilege('mekuin_app','public.correlate_mpesa_unmatched_callbacks()','EXECUTE') THEN
    RAISE EXCEPTION 'runtime role cannot trigger safe callback correlation';
  END IF;
  SELECT pg_get_functiondef('public.ingest_mpesa_stk_callback(text,text,text,jsonb)'::regprocedure) INTO fn;
  IF position('mpesa_unmatched_callback_events' in fn)=0 THEN
    RAISE EXCEPTION 'callback ingestion does not quarantine unmatched callbacks';
  END IF;
  SELECT pg_get_functiondef('public.complete_mpesa_dispatch(uuid,boolean,text,text,text)'::regprocedure) INTO fn;
  IF position('correlate_mpesa_unmatched_callbacks' in fn)=0 THEN
    RAISE EXCEPTION 'dispatch completion does not correlate early callbacks';
  END IF;
END $$;
