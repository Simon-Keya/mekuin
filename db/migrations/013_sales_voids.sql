-- +goose Up
-- Minimal immutable sale identity/amount snapshot for transactional voids.
-- Checkout/payment creation is a separate POS increment; only the void fields
-- below are mutable through the runtime role in this increment.
CREATE TABLE sales (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  business_id uuid NOT NULL REFERENCES businesses(id),
  branch_id uuid NOT NULL,
  sale_number text NOT NULL CHECK (length(trim(sale_number)) BETWEEN 1 AND 80),
  status text NOT NULL DEFAULT 'completed' CHECK (status IN ('completed','voided')),
  total_minor bigint NOT NULL CHECK (total_minor >= 0),
  created_by uuid NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  voided_by uuid,
  voided_at timestamptz,
  void_reason text,
  UNIQUE (business_id,id),
  UNIQUE (business_id,sale_number),
  FOREIGN KEY (business_id,branch_id) REFERENCES branches(business_id,id),
  FOREIGN KEY (business_id,created_by) REFERENCES memberships(business_id,user_id),
  FOREIGN KEY (business_id,voided_by) REFERENCES memberships(business_id,user_id),
  CHECK ((status='completed' AND voided_at IS NULL AND voided_by IS NULL AND void_reason IS NULL)
      OR (status='voided' AND voided_at IS NOT NULL AND voided_by IS NOT NULL AND length(trim(void_reason)) BETWEEN 5 AND 500))
);
CREATE INDEX sales_branch_created_idx ON sales(business_id,branch_id,created_at DESC);
ALTER TABLE sales ENABLE ROW LEVEL SECURITY;
ALTER TABLE sales FORCE ROW LEVEL SECURITY;
CREATE POLICY sales_tenant_policy ON sales
  USING (business_id = nullif(current_setting('app.business_id', true), '')::uuid)
  WITH CHECK (business_id = nullif(current_setting('app.business_id', true), '')::uuid);
GRANT SELECT, INSERT ON sales TO mekuin_app;
REVOKE UPDATE ON sales FROM mekuin_app;
GRANT UPDATE (status,voided_by,voided_at,void_reason) ON sales TO mekuin_app;

-- +goose Down
REVOKE UPDATE (status,voided_by,voided_at,void_reason) ON sales FROM mekuin_app;
DROP TABLE IF EXISTS sales;
