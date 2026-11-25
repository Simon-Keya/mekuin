-- +goose Up
-- Receipt replay protection: key scoped to business and PO; application must
-- persist and compare request fingerprints before returning a prior result.
ALTER TABLE purchase_receipts ADD COLUMN idempotency_key text NOT NULL DEFAULT '';
ALTER TABLE purchase_receipts ADD COLUMN request_fingerprint text NOT NULL DEFAULT '';
CREATE UNIQUE INDEX purchase_receipts_idempotency_uq
  ON purchase_receipts(business_id,purchase_order_id,idempotency_key)
  WHERE idempotency_key <> '';

-- +goose Down
DROP INDEX IF EXISTS purchase_receipts_idempotency_uq;
ALTER TABLE purchase_receipts DROP COLUMN IF EXISTS request_fingerprint;
ALTER TABLE purchase_receipts DROP COLUMN IF EXISTS idempotency_key;
