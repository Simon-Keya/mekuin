DO $$
BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_proc WHERE proname='claim_mpesa_dispatch') THEN RAISE EXCEPTION 'claim_mpesa_dispatch missing'; END IF;
 IF NOT EXISTS (SELECT 1 FROM pg_proc WHERE proname='complete_mpesa_dispatch') THEN RAISE EXCEPTION 'complete_mpesa_dispatch missing'; END IF;
 IF has_table_privilege('mekuin_app','mpesa_dispatch_outbox','SELECT') OR
    has_table_privilege('mekuin_app','mpesa_dispatch_outbox','UPDATE') OR
    has_table_privilege('mekuin_app','mpesa_dispatch_outbox','DELETE') THEN
  RAISE EXCEPTION 'runtime role has direct dispatch outbox access';
 END IF;
 IF NOT has_function_privilege('mekuin_app','claim_mpesa_dispatch()','EXECUTE') OR
    NOT has_function_privilege('mekuin_app','complete_mpesa_dispatch(uuid,boolean,text,text,text)','EXECUTE') THEN
  RAISE EXCEPTION 'runtime role cannot execute dispatch worker functions';
 END IF;
 IF NOT has_column_privilege('mekuin_app','mpesa_dispatch_outbox','attempt_id','INSERT') THEN
  RAISE EXCEPTION 'runtime role cannot enqueue a dispatch';
 END IF;
END $$;
