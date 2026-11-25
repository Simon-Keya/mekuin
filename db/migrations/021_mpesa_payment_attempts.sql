-- +goose Up
-- Durable provider correlation for Daraja STK attempts. This migration is
-- deliberately additive: it does not enable M-Pesa checkout or change sale
-- completion/stock semantics. Those require the verified-settlement workflow.
ALTER TABLE sales DROP CONSTRAINT IF EXISTS sales_status_check;
ALTER TABLE sales DROP CONSTRAINT IF EXISTS sales_check;
ALTER TABLE sales ADD CONSTRAINT sales_status_check CHECK (status IN ('pending_payment','completed','voided'));
ALTER TABLE sales ADD CONSTRAINT sales_lifecycle_check CHECK (
 (status='pending_payment' AND voided_at IS NULL AND voided_by IS NULL AND void_reason IS NULL) OR
 (status='completed' AND voided_at IS NULL AND voided_by IS NULL AND void_reason IS NULL) OR
 (status='voided' AND voided_at IS NOT NULL AND voided_by IS NOT NULL AND length(trim(void_reason)) BETWEEN 5 AND 500)
);

CREATE TABLE mpesa_payment_attempts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 business_id uuid NOT NULL,
 branch_id uuid NOT NULL,
 sale_id uuid NOT NULL,
 sale_payment_id uuid NOT NULL,
 environment text NOT NULL CHECK (environment IN ('sandbox','production')),
 phone_e164 text NOT NULL CHECK (phone_e164 ~ '^254[0-9]{9}$'),
 amount_minor bigint NOT NULL CHECK (amount_minor > 0),
 currency char(3) NOT NULL,
 account_reference text NOT NULL CHECK (length(account_reference) BETWEEN 1 AND 12),
 merchant_request_id text,
 checkout_request_id text,
 status text NOT NULL DEFAULT 'created' CHECK (status IN ('created','accepted','callback_received','verification_pending','verified','failed','expired','review_required')),
 provider_result_code text,
 provider_result_description text,
 mpesa_receipt_number text,
 transaction_at timestamptz,
 accepted_at timestamptz,
 callback_received_at timestamptz,
 verified_at timestamptz,
 failed_at timestamptz,
 created_by uuid NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY (business_id,branch_id,sale_id) REFERENCES sales(business_id,branch_id,id),
 FOREIGN KEY (business_id,sale_payment_id) REFERENCES sale_payments(business_id,id),
 FOREIGN KEY (business_id,created_by) REFERENCES memberships(business_id,user_id),
 UNIQUE (business_id,id),
 UNIQUE (business_id,sale_payment_id),
 CHECK ((status <> 'verified') OR (mpesa_receipt_number IS NOT NULL AND verified_at IS NOT NULL)),
 CHECK ((checkout_request_id IS NULL) OR length(checkout_request_id) BETWEEN 1 AND 128)
);
CREATE UNIQUE INDEX mpesa_attempt_checkout_request_unique
 ON mpesa_payment_attempts(checkout_request_id) WHERE checkout_request_id IS NOT NULL;
CREATE INDEX mpesa_attempt_reconcile_idx
 ON mpesa_payment_attempts(status,created_at) WHERE status IN ('accepted','callback_received','verification_pending','review_required');
ALTER TABLE mpesa_payment_attempts ENABLE ROW LEVEL SECURITY;
ALTER TABLE mpesa_payment_attempts FORCE ROW LEVEL SECURITY;
CREATE POLICY mpesa_payment_attempts_tenant_policy ON mpesa_payment_attempts
 USING (business_id=nullif(current_setting('app.business_id',true),'')::uuid)
 WITH CHECK (business_id=nullif(current_setting('app.business_id',true),'')::uuid);
GRANT SELECT,INSERT ON mpesa_payment_attempts TO mekuin_app;
GRANT UPDATE (merchant_request_id,checkout_request_id,status,provider_result_code,provider_result_description,mpesa_receipt_number,transaction_at,accepted_at,callback_received_at,verified_at,failed_at,updated_at) ON mpesa_payment_attempts TO mekuin_app;

CREATE TABLE mpesa_callback_events (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 business_id uuid NOT NULL,
 attempt_id uuid NOT NULL,
 event_fingerprint char(64) NOT NULL,
 payload jsonb NOT NULL,
 received_at timestamptz NOT NULL DEFAULT now(),
 processed_at timestamptz,
 processing_error text,
 FOREIGN KEY (business_id,attempt_id) REFERENCES mpesa_payment_attempts(business_id,id),
 UNIQUE (business_id,event_fingerprint)
);
CREATE INDEX mpesa_callback_attempt_idx ON mpesa_callback_events(business_id,attempt_id,received_at DESC);
ALTER TABLE mpesa_callback_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE mpesa_callback_events FORCE ROW LEVEL SECURITY;
CREATE POLICY mpesa_callback_events_tenant_policy ON mpesa_callback_events
 USING (business_id=nullif(current_setting('app.business_id',true),'')::uuid)
 WITH CHECK (business_id=nullif(current_setting('app.business_id',true),'')::uuid);
GRANT SELECT,INSERT,UPDATE (processed_at,processing_error) ON mpesa_callback_events TO mekuin_app;

-- +goose Down
DROP TABLE IF EXISTS mpesa_callback_events;
DROP TABLE IF EXISTS mpesa_payment_attempts;
ALTER TABLE sales DROP CONSTRAINT IF EXISTS sales_lifecycle_check;
ALTER TABLE sales DROP CONSTRAINT IF EXISTS sales_status_check;
ALTER TABLE sales ADD CONSTRAINT sales_status_check CHECK (status IN ('completed','voided'));
ALTER TABLE sales ADD CONSTRAINT sales_check CHECK (
 (status='completed' AND voided_at IS NULL AND voided_by IS NULL AND void_reason IS NULL) OR
 (status='voided' AND voided_at IS NOT NULL AND voided_by IS NOT NULL AND length(trim(void_reason)) BETWEEN 5 AND 500)
);
