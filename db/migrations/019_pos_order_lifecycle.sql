-- +goose Up
-- A physical table can have only one active order in a branch at a time.
CREATE UNIQUE INDEX pos_orders_one_active_table_idx
 ON pos_orders(business_id,branch_id,table_id)
 WHERE table_id IS NOT NULL AND status IN ('open','held');

-- +goose Down
DROP INDEX IF EXISTS pos_orders_one_active_table_idx;
