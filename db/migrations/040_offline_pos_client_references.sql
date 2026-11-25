-- +goose Up
-- Durable client references make queued POS draft replay safe after lost responses.
ALTER TABLE pos_orders ADD COLUMN client_reference uuid;
ALTER TABLE pos_orders ADD COLUMN client_request_hash text;
ALTER TABLE pos_orders ADD CONSTRAINT pos_orders_client_reference_hash_check
 CHECK ((client_reference IS NULL AND client_request_hash IS NULL) OR
        (client_reference IS NOT NULL AND client_request_hash IS NOT NULL AND client_request_hash ~ '^[0-9a-f]{64}$'));
CREATE UNIQUE INDEX pos_orders_client_reference_unique
 ON pos_orders(business_id,branch_id,client_reference) WHERE client_reference IS NOT NULL;
-- +goose Down
DROP INDEX IF EXISTS pos_orders_client_reference_unique;
ALTER TABLE pos_orders DROP CONSTRAINT IF EXISTS pos_orders_client_reference_hash_check;
ALTER TABLE pos_orders DROP COLUMN IF EXISTS client_request_hash;
ALTER TABLE pos_orders DROP COLUMN IF EXISTS client_reference;
