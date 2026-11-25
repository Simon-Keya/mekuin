-- +goose Up
-- Enables the final atomic settlement transition. Pending sales are only
-- created by the forthcoming M-Pesa checkout orchestration; cash checkout is
-- unchanged. The source order link is nullable for direct POS sales.
ALTER TABLE sales ADD COLUMN source_order_id uuid;
ALTER TABLE sales ADD CONSTRAINT sales_source_order_fk
 FOREIGN KEY (business_id,source_order_id) REFERENCES pos_orders(business_id,id);
CREATE INDEX sales_source_order_idx ON sales(business_id,source_order_id) WHERE source_order_id IS NOT NULL;
GRANT UPDATE (status) ON sales TO mekuin_app;
GRANT UPDATE (status,provider_reference) ON sale_payments TO mekuin_app;
GRANT UPDATE (status,verified_at,updated_at) ON mpesa_payment_attempts TO mekuin_app;

-- +goose Down
REVOKE UPDATE (status,verified_at,updated_at) ON mpesa_payment_attempts FROM mekuin_app;
REVOKE UPDATE (status,provider_reference) ON sale_payments FROM mekuin_app;
REVOKE UPDATE (status) ON sales FROM mekuin_app;
DROP INDEX IF EXISTS sales_source_order_idx;
ALTER TABLE sales DROP CONSTRAINT IF EXISTS sales_source_order_fk;
ALTER TABLE sales DROP COLUMN IF EXISTS source_order_id;
