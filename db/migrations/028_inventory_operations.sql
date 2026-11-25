-- +goose Up
-- Phase 4A: immutable ledger support for stock losses and inter-branch transfers.
ALTER TABLE inventory_movements DROP CONSTRAINT inventory_movements_movement_type_check;
ALTER TABLE inventory_movements ADD CONSTRAINT inventory_movements_movement_type_check
 CHECK(movement_type IN ('opening','count_reconciliation','sale_consumption','wastage','breakage','transfer_out','transfer_in'));
CREATE INDEX inventory_movements_reference_idx ON inventory_movements(business_id,reference_id) WHERE reference_id IS NOT NULL;
CREATE UNIQUE INDEX inventory_transfer_movement_once_uq
 ON inventory_movements(business_id,reference_id,branch_id,product_id,movement_type)
 WHERE movement_type IN ('transfer_out','transfer_in') AND reference_id IS NOT NULL;
COMMENT ON TABLE inventory_movements IS 'Immutable stock ledger. On-hand is derived as the sum of quantity_delta; corrections are compensating movements.';

-- +goose Down
DROP INDEX IF EXISTS inventory_transfer_movement_once_uq;
DROP INDEX IF EXISTS inventory_movements_reference_idx;
ALTER TABLE inventory_movements DROP CONSTRAINT inventory_movements_movement_type_check;
ALTER TABLE inventory_movements ADD CONSTRAINT inventory_movements_movement_type_check
 CHECK(movement_type IN ('opening','count_reconciliation','sale_consumption'));
