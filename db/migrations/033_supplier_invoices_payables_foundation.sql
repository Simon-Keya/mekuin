-- +goose Up
-- Phase 4C foundation: supplier invoices and payment allocation records.
-- This migration creates records only. It does not auto-create liabilities,
-- post accounting journals, or treat a stock receipt as a supplier payment.

CREATE TABLE supplier_invoices (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  business_id uuid NOT NULL,
  branch_id uuid NOT NULL,
  supplier_id uuid NOT NULL,
  purchase_order_id uuid,
  invoice_number text NOT NULL CHECK (length(trim(invoice_number)) BETWEEN 1 AND 100),
  invoice_date date NOT NULL,
  due_date date,
  currency char(3) NOT NULL CHECK (btrim(currency) ~ '^[A-Z]{3}$'),
  total_minor bigint NOT NULL CHECK (total_minor >= 0),
  status text NOT NULL DEFAULT 'draft'
    CHECK (status IN ('draft','recorded','partially_paid','paid','void')),
  supplier_reference text,
  notes text,
  created_by uuid NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (business_id,id),
  UNIQUE (business_id,supplier_id,invoice_number),
  FOREIGN KEY (business_id,branch_id) REFERENCES branches(business_id,id),
  FOREIGN KEY (business_id,supplier_id) REFERENCES suppliers(business_id,id),
  FOREIGN KEY (business_id,purchase_order_id) REFERENCES purchase_orders(business_id,id),
  FOREIGN KEY (business_id,created_by) REFERENCES memberships(business_id,user_id),
  CHECK (due_date IS NULL OR due_date >= invoice_date)
);
CREATE INDEX supplier_invoices_branch_status_date_idx
  ON supplier_invoices(business_id,branch_id,status,invoice_date DESC);
CREATE INDEX supplier_invoices_supplier_date_idx
  ON supplier_invoices(business_id,supplier_id,invoice_date DESC);
ALTER TABLE supplier_invoices ENABLE ROW LEVEL SECURITY;
ALTER TABLE supplier_invoices FORCE ROW LEVEL SECURITY;
CREATE POLICY supplier_invoices_tenant_policy ON supplier_invoices
  USING (business_id=nullif(current_setting('app.business_id',true),'')::uuid)
  WITH CHECK (business_id=nullif(current_setting('app.business_id',true),'')::uuid);
REVOKE UPDATE,DELETE ON supplier_invoices FROM mekuin_app;
GRANT SELECT,INSERT ON supplier_invoices TO mekuin_app;

CREATE TABLE supplier_invoice_lines (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  business_id uuid NOT NULL,
  invoice_id uuid NOT NULL,
  purchase_order_line_id uuid,
  product_id uuid NOT NULL,
  description text NOT NULL CHECK (length(trim(description)) BETWEEN 1 AND 240),
  quantity numeric(20,8) NOT NULL CHECK (quantity > 0),
  unit_cost_minor bigint NOT NULL CHECK (unit_cost_minor >= 0),
  line_total_minor bigint NOT NULL CHECK (line_total_minor >= 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (business_id,id),
  FOREIGN KEY (business_id,invoice_id) REFERENCES supplier_invoices(business_id,id),
  FOREIGN KEY (business_id,purchase_order_line_id) REFERENCES purchase_order_lines(business_id,id),
  FOREIGN KEY (business_id,product_id) REFERENCES products(business_id,id)
);
CREATE INDEX supplier_invoice_lines_invoice_idx
  ON supplier_invoice_lines(business_id,invoice_id,created_at);
ALTER TABLE supplier_invoice_lines ENABLE ROW LEVEL SECURITY;
ALTER TABLE supplier_invoice_lines FORCE ROW LEVEL SECURITY;
CREATE POLICY supplier_invoice_lines_tenant_policy ON supplier_invoice_lines
  USING (business_id=nullif(current_setting('app.business_id',true),'')::uuid)
  WITH CHECK (business_id=nullif(current_setting('app.business_id',true),'')::uuid);
REVOKE UPDATE,DELETE ON supplier_invoice_lines FROM mekuin_app;
GRANT SELECT,INSERT ON supplier_invoice_lines TO mekuin_app;

-- Payments are explicit supplier disbursements. Allocations connect a payment
-- to one or more invoices; application logic must enforce remaining balances.
CREATE TABLE supplier_payments (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  business_id uuid NOT NULL,
  branch_id uuid NOT NULL,
  supplier_id uuid NOT NULL,
  payment_reference text NOT NULL CHECK (length(trim(payment_reference)) BETWEEN 1 AND 120),
  payment_date date NOT NULL,
  amount_minor bigint NOT NULL CHECK (amount_minor > 0),
  currency char(3) NOT NULL CHECK (btrim(currency) ~ '^[A-Z]{3}$'),
  payment_method text NOT NULL CHECK (payment_method IN ('cash','bank_transfer','card','mobile_money','other')),
  external_reference text,
  notes text,
  recorded_by uuid NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (business_id,id),
  UNIQUE (business_id,payment_reference),
  FOREIGN KEY (business_id,branch_id) REFERENCES branches(business_id,id),
  FOREIGN KEY (business_id,supplier_id) REFERENCES suppliers(business_id,id),
  FOREIGN KEY (business_id,recorded_by) REFERENCES memberships(business_id,user_id)
);
CREATE INDEX supplier_payments_supplier_date_idx
  ON supplier_payments(business_id,supplier_id,payment_date DESC);
ALTER TABLE supplier_payments ENABLE ROW LEVEL SECURITY;
ALTER TABLE supplier_payments FORCE ROW LEVEL SECURITY;
CREATE POLICY supplier_payments_tenant_policy ON supplier_payments
  USING (business_id=nullif(current_setting('app.business_id',true),'')::uuid)
  WITH CHECK (business_id=nullif(current_setting('app.business_id',true),'')::uuid);
REVOKE UPDATE,DELETE ON supplier_payments FROM mekuin_app;
GRANT SELECT,INSERT ON supplier_payments TO mekuin_app;

CREATE TABLE supplier_payment_allocations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  business_id uuid NOT NULL,
  payment_id uuid NOT NULL,
  invoice_id uuid NOT NULL,
  amount_minor bigint NOT NULL CHECK (amount_minor > 0),
  allocated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (business_id,id),
  UNIQUE (business_id,payment_id,invoice_id),
  FOREIGN KEY (business_id,payment_id) REFERENCES supplier_payments(business_id,id),
  FOREIGN KEY (business_id,invoice_id) REFERENCES supplier_invoices(business_id,id)
);
CREATE INDEX supplier_payment_allocations_invoice_idx
  ON supplier_payment_allocations(business_id,invoice_id,allocated_at);
ALTER TABLE supplier_payment_allocations ENABLE ROW LEVEL SECURITY;
ALTER TABLE supplier_payment_allocations FORCE ROW LEVEL SECURITY;
CREATE POLICY supplier_payment_allocations_tenant_policy ON supplier_payment_allocations
  USING (business_id=nullif(current_setting('app.business_id',true),'')::uuid)
  WITH CHECK (business_id=nullif(current_setting('app.business_id',true),'')::uuid);
REVOKE UPDATE,DELETE ON supplier_payment_allocations FROM mekuin_app;
GRANT SELECT,INSERT ON supplier_payment_allocations TO mekuin_app;

-- +goose Down
DROP TABLE IF EXISTS supplier_payment_allocations;
DROP TABLE IF EXISTS supplier_payments;
DROP TABLE IF EXISTS supplier_invoice_lines;
DROP TABLE IF EXISTS supplier_invoices;
