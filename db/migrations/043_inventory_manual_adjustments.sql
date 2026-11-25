-- +goose Up
-- Manual adjustments are explicit, reasoned ledger entries; the ledger remains append-only.
ALTER TABLE inventory_movements DROP CONSTRAINT inventory_movements_movement_type_check;
ALTER TABLE inventory_movements ADD CONSTRAINT inventory_movements_movement_type_check
 CHECK(movement_type IN ('opening','count_reconciliation','sale_consumption','wastage','breakage','transfer_out','transfer_in','purchase_receipt','manual_adjustment'));

-- +goose Down
ALTER TABLE inventory_movements DROP CONSTRAINT inventory_movements_movement_type_check;
ALTER TABLE inventory_movements ADD CONSTRAINT inventory_movements_movement_type_check
 CHECK(movement_type IN ('opening','count_reconciliation','sale_consumption','wastage','breakage','transfer_out','transfer_in','purchase_receipt'));
