-- +goose Up
-- Transactional refunds, inventory count reconciliation and durable email outbox.
ALTER TABLE sales ADD CONSTRAINT sales_business_branch_id_uq UNIQUE(business_id,branch_id,id);
CREATE TABLE sale_refunds (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), business_id uuid NOT NULL, branch_id uuid NOT NULL,
 sale_id uuid NOT NULL, amount_minor bigint NOT NULL CHECK(amount_minor > 0), reason text NOT NULL CHECK(length(trim(reason)) BETWEEN 5 AND 500),
 refunded_by uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(business_id,branch_id,sale_id) REFERENCES sales(business_id,branch_id,id),
 FOREIGN KEY(business_id,branch_id) REFERENCES branches(business_id,id),
 FOREIGN KEY(business_id,refunded_by) REFERENCES memberships(business_id,user_id),
 UNIQUE(business_id,id)
);
CREATE INDEX sale_refunds_sale_idx ON sale_refunds(business_id,sale_id);
ALTER TABLE sale_refunds ENABLE ROW LEVEL SECURITY; ALTER TABLE sale_refunds FORCE ROW LEVEL SECURITY;
CREATE POLICY sale_refunds_tenant_policy ON sale_refunds USING(business_id=nullif(current_setting('app.business_id',true),'')::uuid) WITH CHECK(business_id=nullif(current_setting('app.business_id',true),'')::uuid);
REVOKE UPDATE,DELETE ON sale_refunds FROM mekuin_app;
GRANT SELECT,INSERT ON sale_refunds TO mekuin_app;

CREATE TABLE inventory_movements (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), business_id uuid NOT NULL, branch_id uuid NOT NULL,
 product_id uuid NOT NULL, quantity_delta numeric(20,8) NOT NULL CHECK(quantity_delta<>0),
 movement_type text NOT NULL CHECK(movement_type IN ('opening','count_reconciliation')),
 reference_id uuid, created_by uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(business_id,branch_id) REFERENCES branches(business_id,id),
 FOREIGN KEY(business_id,product_id) REFERENCES products(business_id,id),
 FOREIGN KEY(business_id,created_by) REFERENCES memberships(business_id,user_id)
);
CREATE INDEX inventory_movements_balance_idx ON inventory_movements(business_id,branch_id,product_id,created_at,id);
ALTER TABLE inventory_movements ENABLE ROW LEVEL SECURITY; ALTER TABLE inventory_movements FORCE ROW LEVEL SECURITY;
CREATE POLICY inventory_movements_tenant_policy ON inventory_movements USING(business_id=nullif(current_setting('app.business_id',true),'')::uuid) WITH CHECK(business_id=nullif(current_setting('app.business_id',true),'')::uuid);
REVOKE UPDATE,DELETE ON inventory_movements FROM mekuin_app;
GRANT SELECT,INSERT ON inventory_movements TO mekuin_app;

CREATE TABLE inventory_counts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), business_id uuid NOT NULL, branch_id uuid NOT NULL,
 status text NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','reconciled')),
 created_by uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), reconciled_by uuid, reconciled_at timestamptz,
 FOREIGN KEY(business_id,branch_id) REFERENCES branches(business_id,id),
 FOREIGN KEY(business_id,created_by) REFERENCES memberships(business_id,user_id),
 FOREIGN KEY(business_id,reconciled_by) REFERENCES memberships(business_id,user_id),
 UNIQUE(business_id,id)
);
CREATE UNIQUE INDEX inventory_counts_one_draft_branch_uq ON inventory_counts(business_id,branch_id) WHERE status='draft';
CREATE TABLE inventory_count_lines (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), business_id uuid NOT NULL, count_id uuid NOT NULL,
 product_id uuid NOT NULL, expected_quantity numeric(20,8) NOT NULL, counted_quantity numeric(20,8) NOT NULL CHECK(counted_quantity>=0),
 UNIQUE(business_id,count_id,product_id),
 FOREIGN KEY(business_id,count_id) REFERENCES inventory_counts(business_id,id),
 FOREIGN KEY(business_id,product_id) REFERENCES products(business_id,id)
);
ALTER TABLE inventory_counts ENABLE ROW LEVEL SECURITY; ALTER TABLE inventory_counts FORCE ROW LEVEL SECURITY;
ALTER TABLE inventory_count_lines ENABLE ROW LEVEL SECURITY; ALTER TABLE inventory_count_lines FORCE ROW LEVEL SECURITY;
CREATE POLICY inventory_counts_tenant_policy ON inventory_counts USING(business_id=nullif(current_setting('app.business_id',true),'')::uuid) WITH CHECK(business_id=nullif(current_setting('app.business_id',true),'')::uuid);
CREATE POLICY inventory_count_lines_tenant_policy ON inventory_count_lines USING(business_id=nullif(current_setting('app.business_id',true),'')::uuid) WITH CHECK(business_id=nullif(current_setting('app.business_id',true),'')::uuid);
REVOKE UPDATE,DELETE ON inventory_counts FROM mekuin_app;
REVOKE UPDATE,DELETE ON inventory_count_lines FROM mekuin_app;
REVOKE INSERT ON inventory_counts FROM mekuin_app;
GRANT SELECT ON inventory_counts TO mekuin_app;
GRANT INSERT(business_id,branch_id,created_by) ON inventory_counts TO mekuin_app;
GRANT UPDATE(status,reconciled_by,reconciled_at) ON inventory_counts TO mekuin_app;
REVOKE INSERT ON inventory_count_lines FROM mekuin_app;
GRANT SELECT,INSERT(business_id,count_id,product_id,expected_quantity,counted_quantity) ON inventory_count_lines TO mekuin_app;

-- Global worker queue: only invitation creation can enqueue; API worker claims and updates.
-- Payload is cleared after successful delivery and bounded retries prevent poison loops.
CREATE TABLE email_outbox (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), business_id uuid NOT NULL,
 recipient text NOT NULL, subject text NOT NULL, body text NOT NULL,
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','sending','sent','failed')),
 attempts integer NOT NULL DEFAULT 0 CHECK(attempts>=0), available_at timestamptz NOT NULL DEFAULT now(),
 locked_at timestamptz, sent_at timestamptz, last_error text, created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(business_id) REFERENCES businesses(id)
);
CREATE INDEX email_outbox_pending_idx ON email_outbox(available_at,created_at) WHERE status IN ('pending','sending');
ALTER TABLE email_outbox ENABLE ROW LEVEL SECURITY;
CREATE POLICY email_outbox_enqueue_policy ON email_outbox FOR INSERT
 WITH CHECK(business_id=nullif(current_setting('app.business_id',true),'')::uuid);
REVOKE SELECT,UPDATE,DELETE ON email_outbox FROM mekuin_app;
GRANT INSERT ON email_outbox TO mekuin_app;

-- Worker access is mediated through narrow SECURITY DEFINER functions. The
-- runtime role cannot read or mutate queued messages directly.
CREATE FUNCTION claim_email_outbox()
RETURNS TABLE(id uuid,recipient text,subject text,body text,attempts integer)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
 WITH candidate AS (
  SELECT e.id FROM public.email_outbox e
  WHERE ((e.status='pending' AND e.available_at<=now()) OR (e.status='sending' AND e.locked_at<now()-interval '5 minutes'))
  ORDER BY e.created_at FOR UPDATE SKIP LOCKED LIMIT 1
 )
 UPDATE public.email_outbox e SET status='sending',locked_at=now(),attempts=e.attempts+1
 FROM candidate c WHERE e.id=c.id
 RETURNING e.id,e.recipient,e.subject,e.body,e.attempts;
$$;
CREATE FUNCTION complete_email_outbox(p_id uuid,p_success boolean,p_error text,p_delay_seconds integer)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
BEGIN
 IF p_success THEN
  UPDATE public.email_outbox SET status='sent',sent_at=now(),locked_at=NULL,last_error=NULL,body=''
   WHERE id=p_id AND status='sending';
 ELSE
  UPDATE public.email_outbox SET status=CASE WHEN attempts>=8 THEN 'failed' ELSE 'pending' END,
   available_at=now()+make_interval(secs=>greatest(1,least(p_delay_seconds,3600))),locked_at=NULL,
   last_error=left(COALESCE(p_error,'delivery failed'),500),body=CASE WHEN attempts>=8 THEN '' ELSE body END
   WHERE id=p_id AND status='sending';
 END IF;
END $$;
REVOKE ALL ON FUNCTION claim_email_outbox() FROM PUBLIC;
REVOKE ALL ON FUNCTION complete_email_outbox(uuid,boolean,text,integer) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION claim_email_outbox() TO mekuin_app;
GRANT EXECUTE ON FUNCTION complete_email_outbox(uuid,boolean,text,integer) TO mekuin_app;

-- +goose Down
DROP FUNCTION IF EXISTS complete_email_outbox(uuid,boolean,text,integer);
DROP FUNCTION IF EXISTS claim_email_outbox();
DROP TABLE IF EXISTS email_outbox;
DROP TABLE IF EXISTS inventory_count_lines;
DROP TABLE IF EXISTS inventory_counts;
DROP TABLE IF EXISTS inventory_movements;
DROP TABLE IF EXISTS sale_refunds;
ALTER TABLE sales DROP CONSTRAINT IF EXISTS sales_business_branch_id_uq;
