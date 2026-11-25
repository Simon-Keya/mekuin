-- +goose Up
-- Callback ingestion is a narrow SECURITY DEFINER operation because Daraja
-- callbacks are unauthenticated provider-to-provider requests and the runtime
-- role must not receive cross-tenant table access. It only stores the event and
-- advances an attempt to callback_received; it never settles a payment.
CREATE FUNCTION ingest_mpesa_stk_callback(
 p_checkout_request_id text,
 p_merchant_request_id text,
 p_fingerprint text,
 p_payload jsonb
) RETURNS TABLE(business_id uuid,attempt_id uuid,duplicate boolean)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
DECLARE
 v_business_id uuid;
 v_attempt_id uuid;
 v_status text;
 v_inserted integer;
BEGIN
 IF p_checkout_request_id IS NULL OR length(p_checkout_request_id) NOT BETWEEN 1 AND 128
    OR p_merchant_request_id IS NULL OR length(p_merchant_request_id) NOT BETWEEN 1 AND 128
    OR p_fingerprint !~ '^[0-9a-f]{64}$'
    OR p_payload IS NULL OR jsonb_typeof(p_payload) <> 'object' THEN
   RETURN;
 END IF;
 SELECT a.business_id,a.id,a.status INTO v_business_id,v_attempt_id,v_status
 FROM public.mpesa_payment_attempts a
 WHERE a.checkout_request_id=p_checkout_request_id
   AND a.merchant_request_id=p_merchant_request_id
 FOR UPDATE;
 IF NOT FOUND THEN RETURN; END IF;
 INSERT INTO public.mpesa_callback_events(business_id,attempt_id,event_fingerprint,payload)
 VALUES(v_business_id,v_attempt_id,p_fingerprint,p_payload)
 ON CONFLICT (business_id,event_fingerprint) DO NOTHING;
 GET DIAGNOSTICS v_inserted = ROW_COUNT;
 IF v_inserted=1 AND v_status IN ('accepted','callback_received','verification_pending') THEN
   UPDATE public.mpesa_payment_attempts
   SET status='callback_received',callback_received_at=COALESCE(callback_received_at,now()),updated_at=now()
   WHERE business_id=v_business_id AND id=v_attempt_id;
 END IF;
 RETURN QUERY SELECT v_business_id,v_attempt_id,(v_inserted=0);
END;
$$;
REVOKE ALL ON FUNCTION ingest_mpesa_stk_callback(text,text,text,jsonb) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ingest_mpesa_stk_callback(text,text,text,jsonb) TO mekuin_app;

-- +goose Down
REVOKE EXECUTE ON FUNCTION ingest_mpesa_stk_callback(text,text,text,jsonb) FROM mekuin_app;
DROP FUNCTION IF EXISTS ingest_mpesa_stk_callback(text,text,text,jsonb);
