-- +goose Up
-- Recipe-backed checkout consumption is an immutable ledger movement.
ALTER TABLE inventory_movements DROP CONSTRAINT inventory_movements_movement_type_check;
ALTER TABLE inventory_movements ADD CONSTRAINT inventory_movements_movement_type_check
 CHECK(movement_type IN ('opening','count_reconciliation','sale_consumption'));
CREATE UNIQUE INDEX inventory_movements_sale_consumption_uq
 ON inventory_movements(business_id,branch_id,reference_id,product_id)
 WHERE movement_type='sale_consumption' AND reference_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS inventory_movements_sale_consumption_uq;
ALTER TABLE inventory_movements DROP CONSTRAINT inventory_movements_movement_type_check;
ALTER TABLE inventory_movements ADD CONSTRAINT inventory_movements_movement_type_check
 CHECK(movement_type IN ('opening','count_reconciliation'));
