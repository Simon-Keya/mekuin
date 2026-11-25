-- +goose Up
-- Preserve valid callbacks that arrive before the dispatch worker has committed
-- Safaricom's request identifiers. A restricted inbox plus a correlation
-- function closes the post-commit dispatch/callback timing race.
CREATE TABLE mpesa_unmatched_callback_events (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 event_fingerprint char(64) NOT NULL UNIQUE CHECK(event_fingerprint ~ '^[0-9a-f]{64}$'),
 checkout_request_id text NOT NULL CHECK(length(checkout_request_id) BETWEEN 1 AND 128),
 merchant_request_id text NOT NULL CHECK(length(merchant_request_id) BETWEEN 1 AND 128),
 payload jsonb NOT NULL CHECK(jsonb_typeof(payload)='object'),
 received_at timestamptz NOT NULL DEFAULT now(),
 correlated_at timestamptz,
 correlated_business_id uuid,
 correlated_attempt_id uuid
);
CREATE INDEX mpesa_unmatched_callback_lookup_idx
 ON mpesa_unmatched_callback_events(checkout_request_id,merchant_request_id)
 WHERE correlated_at IS NULL;
REVOKE ALL ON mpesa_unmatched_callback_events FROM PUBLIC,mekuin_app;

CREATE OR REPLACE FUNCTION correlate_mpesa_unmatched_callbacks()
RETURNS integer LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
DECLARE v_count integer := 0;
BEGIN
 WITH candidates AS (
  SELECT u.id AS inbox_id,u.event_fingerprint,u.payload,a.business_id,a.id AS attempt_id,a.status
  FROM public.mpesa_unmatched_callback_events u
  JOIN public.mpesa_payment_attempts a
   ON a.checkout_request_id=u.checkout_request_id
   AND a.merchant_request_id=u.merchant_request_id
  WHERE u.correlated_at IS NULL
  ORDER BY u.received_at
  FOR UPDATE OF u SKIP LOCKED
 ), inserted AS (
  INSERT INTO public.mpesa_callback_events(business_id,attempt_id,event_fingerprint,payload)
  SELECT business_id,attempt_id,event_fingerprint,payload FROM candidates
  ON CONFLICT (business_id,event_fingerprint) DO NOTHING
  RETURNING business_id,attempt_id,event_fingerprint
 ), marked AS (
  UPDATE public.mpesa_unmatched_callback_events u
  SET correlated_at=now(),correlated_business_id=c.business_id,correlated_attempt_id=c.attempt_id
  FROM candidates c WHERE u.id=c.inbox_id
  RETURNING c.business_id,c.attempt_id,c.status,c.payload
 )
 SELECT count(*) INTO v_count FROM marked;
 UPDATE public.mpesa_payment_attempts a
 SET status=CASE
   WHEN m.payload #>> '{Body,stkCallback,ResultCode}'='0'
    AND a.status IN ('failed','expired','review_required') THEN 'review_required'
   WHEN a.status IN ('accepted','callback_received','verification_pending') THEN 'callback_received'
   ELSE a.status END,
  callback_received_at=COALESCE(a.callback_received_at,now()),updated_at=now()
 FROM (
  SELECT DISTINCT correlated_business_id AS business_id,correlated_attempt_id AS attempt_id,
   payload FROM public.mpesa_unmatched_callback_events
  WHERE correlated_at IS NOT NULL AND correlated_at >= now()-interval '2 minutes'
 ) m
 WHERE a.business_id=m.business_id AND a.id=m.attempt_id
  AND ((m.payload #>> '{Body,stkCallback,ResultCode}'='0' AND a.status IN ('failed','expired','review_required'))
    OR a.status IN ('accepted','callback_received','verification_pending'));
 RETURN v_count;
END $$;

CREATE OR REPLACE FUNCTION ingest_mpesa_stk_callback(
 p_checkout_request_id text,
 p_merchant_request_id text,
 p_fingerprint text,
 p_payload jsonb
) RETURNS TABLE(business_id uuid,attempt_id uuid,duplicate boolean)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
DECLARE
 v_business_id uuid;
 v_attempt_id uuid;
 v_inserted integer;
BEGIN
 IF p_checkout_request_id IS NULL OR length(p_checkout_request_id) NOT BETWEEN 1 AND 128
    OR p_merchant_request_id IS NULL OR length(p_merchant_request_id) NOT BETWEEN 1 AND 128
    OR p_fingerprint !~ '^[0-9a-f]{64}$'
    OR p_payload IS NULL OR jsonb_typeof(p_payload) <> 'object' THEN
   RETURN;
 END IF;
 SELECT a.business_id,a.id INTO v_business_id,v_attempt_id
 FROM public.mpesa_payment_attempts a
 WHERE a.checkout_request_id=p_checkout_request_id
   AND a.merchant_request_id=p_merchant_request_id
 FOR UPDATE;
 IF NOT FOUND THEN
  INSERT INTO public.mpesa_unmatched_callback_events(event_fingerprint,checkout_request_id,merchant_request_id,payload)
  VALUES(p_fingerprint,p_checkout_request_id,p_merchant_request_id,p_payload)
  ON CONFLICT(event_fingerprint) DO NOTHING;
  PERFORM public.correlate_mpesa_unmatched_callbacks();
  RETURN;
 END IF;
 INSERT INTO public.mpesa_callback_events(business_id,attempt_id,event_fingerprint,payload)
 VALUES(v_business_id,v_attempt_id,p_fingerprint,p_payload)
 ON CONFLICT (business_id,event_fingerprint) DO NOTHING;
 GET DIAGNOSTICS v_inserted = ROW_COUNT;
 IF p_payload #>> '{Body,stkCallback,ResultCode}'='0' THEN
  UPDATE public.mpesa_payment_attempts SET
   status=CASE WHEN status IN ('failed','expired','review_required') THEN 'review_required'
    WHEN status IN ('accepted','callback_received','verification_pending') THEN 'callback_received' ELSE status END,
   callback_received_at=COALESCE(callback_received_at,now()),updated_at=now()
  WHERE business_id=v_business_id AND id=v_attempt_id;
 ELSIF v_inserted=1 THEN
  UPDATE public.mpesa_payment_attempts SET
   status=CASE WHEN status IN ('accepted','callback_received','verification_pending') THEN 'callback_received' ELSE status END,
   callback_received_at=COALESCE(callback_received_at,now()),updated_at=now()
  WHERE business_id=v_business_id AND id=v_attempt_id;
 END IF;
 RETURN QUERY SELECT v_business_id,v_attempt_id,(v_inserted=0);
END;
$$;

-- Dispatch completion and callback ingestion both invoke correlation. This
-- handles either transaction winning the race without a second STK request.
CREATE OR REPLACE FUNCTION complete_mpesa_dispatch(p_attempt_id uuid,p_accepted boolean,p_merchant_request_id text,p_checkout_request_id text,p_error text)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
DECLARE v_business_id uuid;
BEGIN
 SELECT business_id INTO v_business_id FROM public.mpesa_payment_attempts WHERE id=p_attempt_id;
 IF v_business_id IS NULL THEN RETURN; END IF;
 IF p_accepted AND length(coalesce(p_merchant_request_id,'')) BETWEEN 1 AND 128
  AND length(coalesce(p_checkout_request_id,'')) BETWEEN 1 AND 128 THEN
  UPDATE public.mpesa_payment_attempts SET status='accepted',merchant_request_id=p_merchant_request_id,
   checkout_request_id=p_checkout_request_id,accepted_at=now(),updated_at=now()
  WHERE business_id=v_business_id AND id=p_attempt_id AND status='created';
  UPDATE public.mpesa_dispatch_outbox SET status='submitted',completed_at=now(),locked_at=NULL,last_error=NULL
  WHERE business_id=v_business_id AND attempt_id=p_attempt_id AND status='dispatching';
  PERFORM public.correlate_mpesa_unmatched_callbacks();
 ELSE
  UPDATE public.mpesa_payment_attempts SET status='review_required',provider_result_description=left(coalesce(p_error,'invalid_provider_response'),500),updated_at=now()
  WHERE business_id=v_business_id AND id=p_attempt_id AND status='created';
  UPDATE public.mpesa_dispatch_outbox SET status='review_required',completed_at=now(),locked_at=NULL,
   last_error=left(coalesce(p_error,'ambiguous_provider_submission'),500)
  WHERE business_id=v_business_id AND attempt_id=p_attempt_id AND status='dispatching';
 END IF;
END $$;

REVOKE ALL ON FUNCTION correlate_mpesa_unmatched_callbacks() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION correlate_mpesa_unmatched_callbacks() TO mekuin_app;
REVOKE ALL ON FUNCTION ingest_mpesa_stk_callback(text,text,text,jsonb) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ingest_mpesa_stk_callback(text,text,text,jsonb) TO mekuin_app;
REVOKE ALL ON FUNCTION complete_mpesa_dispatch(uuid,boolean,text,text,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION complete_mpesa_dispatch(uuid,boolean,text,text,text) TO mekuin_app;

-- +goose Down
DO $$
BEGIN
 IF EXISTS (SELECT 1 FROM public.mpesa_unmatched_callback_events WHERE correlated_at IS NULL) THEN
  RAISE EXCEPTION 'cannot roll back while unmatched Daraja callbacks remain quarantined';
 END IF;
END $$;

CREATE OR REPLACE FUNCTION ingest_mpesa_stk_callback(
 p_checkout_request_id text,p_merchant_request_id text,p_fingerprint text,p_payload jsonb
) RETURNS TABLE(business_id uuid,attempt_id uuid,duplicate boolean)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
DECLARE v_business_id uuid; v_attempt_id uuid; v_status text; v_inserted integer;
BEGIN
 IF p_checkout_request_id IS NULL OR length(p_checkout_request_id) NOT BETWEEN 1 AND 128
  OR p_merchant_request_id IS NULL OR length(p_merchant_request_id) NOT BETWEEN 1 AND 128
  OR p_fingerprint !~ '^[0-9a-f]{64}$' OR p_payload IS NULL OR jsonb_typeof(p_payload)<>'object' THEN RETURN; END IF;
 SELECT a.business_id,a.id,a.status INTO v_business_id,v_attempt_id,v_status
 FROM public.mpesa_payment_attempts a WHERE a.checkout_request_id=p_checkout_request_id
  AND a.merchant_request_id=p_merchant_request_id FOR UPDATE;
 IF NOT FOUND THEN RETURN; END IF;
 INSERT INTO public.mpesa_callback_events(business_id,attempt_id,event_fingerprint,payload)
 VALUES(v_business_id,v_attempt_id,p_fingerprint,p_payload)
 ON CONFLICT (business_id,event_fingerprint) DO NOTHING;
 GET DIAGNOSTICS v_inserted = ROW_COUNT;
 IF v_inserted=1 AND v_status IN ('accepted','callback_received','verification_pending') THEN
  UPDATE public.mpesa_payment_attempts SET status='callback_received',callback_received_at=COALESCE(callback_received_at,now()),updated_at=now()
  WHERE business_id=v_business_id AND id=v_attempt_id;
 END IF;
 RETURN QUERY SELECT v_business_id,v_attempt_id,(v_inserted=0);
END;
$$;

CREATE OR REPLACE FUNCTION complete_mpesa_dispatch(p_attempt_id uuid,p_accepted boolean,p_merchant_request_id text,p_checkout_request_id text,p_error text)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
DECLARE v_business_id uuid;
BEGIN
 SELECT business_id INTO v_business_id FROM public.mpesa_payment_attempts WHERE id=p_attempt_id;
 IF v_business_id IS NULL THEN RETURN; END IF;
 IF p_accepted AND length(coalesce(p_merchant_request_id,'')) BETWEEN 1 AND 128
  AND length(coalesce(p_checkout_request_id,'')) BETWEEN 1 AND 128 THEN
  UPDATE public.mpesa_payment_attempts SET status='accepted',merchant_request_id=p_merchant_request_id,
   checkout_request_id=p_checkout_request_id,accepted_at=now(),updated_at=now()
  WHERE business_id=v_business_id AND id=p_attempt_id AND status='created';
  UPDATE public.mpesa_dispatch_outbox SET status='submitted',completed_at=now(),locked_at=NULL,last_error=NULL
  WHERE business_id=v_business_id AND attempt_id=p_attempt_id AND status='dispatching';
 ELSE
  UPDATE public.mpesa_payment_attempts SET status='review_required',provider_result_description=left(coalesce(p_error,'invalid_provider_response'),500),updated_at=now()
  WHERE business_id=v_business_id AND id=p_attempt_id AND status='created';
  UPDATE public.mpesa_dispatch_outbox SET status='review_required',completed_at=now(),locked_at=NULL,
   last_error=left(coalesce(p_error,'ambiguous_provider_submission'),500)
  WHERE business_id=v_business_id AND attempt_id=p_attempt_id AND status='dispatching';
 END IF;
END $$;

REVOKE EXECUTE ON FUNCTION correlate_mpesa_unmatched_callbacks() FROM mekuin_app;
DROP FUNCTION IF EXISTS correlate_mpesa_unmatched_callbacks();
DROP TABLE IF EXISTS mpesa_unmatched_callback_events;
