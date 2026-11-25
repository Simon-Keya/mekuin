-- +goose Up
-- STK requests are dispatched only after the sale, tender, attempt and outbox
-- row commit together. Ambiguous submissions are never automatically retried.
CREATE TABLE mpesa_dispatch_outbox (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 business_id uuid NOT NULL,
 branch_id uuid NOT NULL,
 attempt_id uuid NOT NULL,
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','dispatching','submitted','review_required')),
 attempts integer NOT NULL DEFAULT 0 CHECK(attempts BETWEEN 0 AND 1),
 created_at timestamptz NOT NULL DEFAULT now(),
 locked_at timestamptz,
 completed_at timestamptz,
 last_error text,
 FOREIGN KEY(business_id,branch_id) REFERENCES branches(business_id,id),
 FOREIGN KEY(business_id,attempt_id) REFERENCES mpesa_payment_attempts(business_id,id),
 UNIQUE(business_id,attempt_id)
);
CREATE INDEX mpesa_dispatch_pending_idx ON mpesa_dispatch_outbox(created_at) WHERE status='pending';
ALTER TABLE mpesa_dispatch_outbox ENABLE ROW LEVEL SECURITY;

CREATE POLICY mpesa_dispatch_enqueue_policy ON mpesa_dispatch_outbox FOR INSERT
 WITH CHECK(business_id=nullif(current_setting('app.business_id',true),'')::uuid
 AND branch_id=nullif(current_setting('app.branch_id',true),'')::uuid);
REVOKE ALL ON mpesa_dispatch_outbox FROM PUBLIC,mekuin_app;
GRANT INSERT(business_id,branch_id,attempt_id) ON mpesa_dispatch_outbox TO mekuin_app;

CREATE FUNCTION claim_mpesa_dispatch()
RETURNS TABLE(attempt_id uuid,phone_e164 text,amount_minor bigint,account_reference text,environment text)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
DECLARE v_id uuid;
BEGIN
 -- A worker crash after claim may have happened after Safaricom accepted the
 -- request. Such rows require review; they must not be sent a second time.
 UPDATE public.mpesa_dispatch_outbox SET status='review_required',completed_at=now(),
  last_error='worker_claim_expired; provider submission outcome is ambiguous'
 WHERE status='dispatching' AND locked_at < now()-interval '2 minutes';
 UPDATE public.mpesa_payment_attempts a SET status='review_required',updated_at=now()
 FROM public.mpesa_dispatch_outbox o
 WHERE o.attempt_id=a.id AND o.business_id=a.business_id AND o.status='review_required' AND a.status='created';
 SELECT o.id INTO v_id FROM public.mpesa_dispatch_outbox o
 WHERE o.status='pending' ORDER BY o.created_at FOR UPDATE SKIP LOCKED LIMIT 1;
 IF v_id IS NULL THEN RETURN; END IF;
 RETURN QUERY
 WITH claimed AS (
  UPDATE public.mpesa_dispatch_outbox o SET status='dispatching',attempts=1,locked_at=now()
  WHERE o.id=v_id AND o.status='pending'
  RETURNING o.business_id,o.attempt_id
 )
 SELECT a.id,a.phone_e164,a.amount_minor,a.account_reference,a.environment
 FROM claimed c JOIN public.mpesa_payment_attempts a
  ON a.business_id=c.business_id AND a.id=c.attempt_id
 WHERE a.status='created';
END $$;

CREATE FUNCTION complete_mpesa_dispatch(p_attempt_id uuid,p_accepted boolean,p_merchant_request_id text,p_checkout_request_id text,p_error text)
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
REVOKE ALL ON FUNCTION claim_mpesa_dispatch() FROM PUBLIC;
REVOKE ALL ON FUNCTION complete_mpesa_dispatch(uuid,boolean,text,text,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION claim_mpesa_dispatch() TO mekuin_app;
GRANT EXECUTE ON FUNCTION complete_mpesa_dispatch(uuid,boolean,text,text,text) TO mekuin_app;

-- +goose Down
REVOKE EXECUTE ON FUNCTION complete_mpesa_dispatch(uuid,boolean,text,text,text) FROM mekuin_app;
REVOKE EXECUTE ON FUNCTION claim_mpesa_dispatch() FROM mekuin_app;
DROP FUNCTION IF EXISTS complete_mpesa_dispatch(uuid,boolean,text,text,text);
DROP FUNCTION IF EXISTS claim_mpesa_dispatch();
DROP TABLE IF EXISTS mpesa_dispatch_outbox;
