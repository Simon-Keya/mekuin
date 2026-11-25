-- Run as migration owner after all migrations and runtime-role provisioning.
DO $$
DECLARE
  status_check text;
BEGIN
  SELECT pg_get_constraintdef(c.oid) INTO status_check
  FROM pg_constraint c
  WHERE c.conrelid='public.mpesa_payment_attempts'::regclass
    AND c.conname='mpesa_payment_attempts_status_check';
  IF status_check IS NULL OR position('provider_confirmed' in status_check)=0 THEN
    RAISE EXCEPTION 'provider_confirmed is missing from the payment attempt lifecycle';
  END IF;
  IF NOT has_column_privilege('mekuin_app','public.mpesa_payment_attempts','provider_queried_at','UPDATE') THEN
    RAISE EXCEPTION 'runtime role cannot persist provider query timestamps';
  END IF;
  IF NOT has_column_privilege('mekuin_app','public.mpesa_payment_attempts','callback_metadata_matched_at','UPDATE') THEN
    RAISE EXCEPTION 'runtime role cannot persist callback metadata match';
  END IF;
  IF has_table_privilege('mekuin_app','public.mpesa_payment_attempts','DELETE') THEN
    RAISE EXCEPTION 'runtime role must not delete payment attempts';
  END IF;
  IF has_table_privilege('mekuin_app','public.mpesa_callback_events','DELETE') THEN
    RAISE EXCEPTION 'runtime role must not delete callback events';
  END IF;
END $$;
