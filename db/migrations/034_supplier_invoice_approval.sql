-- +goose Up
-- Immutable invoice workflow events. Approval is separate from three-way matching
-- and payment allocation; the latest event determines the current review state.
CREATE TABLE supplier_invoice_review_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  business_id uuid NOT NULL,
  branch_id uuid NOT NULL,
  invoice_id uuid NOT NULL,
  event_no bigint NOT NULL CHECK (event_no > 0),
  event_type text NOT NULL CHECK (event_type IN ('submitted','approved','approved_with_exceptions','rejected')),
  reason text,
  actor_user_id uuid NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (business_id,id),
  UNIQUE (business_id,invoice_id,event_no),
  FOREIGN KEY (business_id,branch_id) REFERENCES branches(business_id,id),
  FOREIGN KEY (business_id,invoice_id) REFERENCES supplier_invoices(business_id,id),
  FOREIGN KEY (business_id,actor_user_id) REFERENCES memberships(business_id,user_id),
  CHECK (reason IS NULL OR length(reason) <= 2000)
);
CREATE INDEX supplier_invoice_review_latest_idx
  ON supplier_invoice_review_events(business_id,branch_id,invoice_id,event_no DESC);
ALTER TABLE supplier_invoice_review_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE supplier_invoice_review_events FORCE ROW LEVEL SECURITY;
CREATE POLICY supplier_invoice_review_events_tenant_policy ON supplier_invoice_review_events
  USING (business_id=nullif(current_setting('app.business_id',true),'')::uuid)
  WITH CHECK (business_id=nullif(current_setting('app.business_id',true),'')::uuid);
REVOKE UPDATE,DELETE ON supplier_invoice_review_events FROM mekuin_app;
GRANT SELECT,INSERT ON supplier_invoice_review_events TO mekuin_app;

-- +goose Down
DROP TABLE IF EXISTS supplier_invoice_review_events;
