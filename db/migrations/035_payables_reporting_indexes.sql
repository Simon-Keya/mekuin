-- +goose Up
-- Supports branch-scoped ageing, supplier statements and payment allocation review.
CREATE INDEX supplier_invoices_branch_currency_due_idx
  ON supplier_invoices(business_id,branch_id,currency,due_date,supplier_id)
  WHERE status <> 'void';

CREATE INDEX supplier_payments_branch_currency_date_idx
  ON supplier_payments(business_id,branch_id,currency,payment_date DESC,supplier_id);

-- +goose Down
DROP INDEX IF EXISTS supplier_payments_branch_currency_date_idx;
DROP INDEX IF EXISTS supplier_invoices_branch_currency_due_idx;
