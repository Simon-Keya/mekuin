-- +goose Up
ALTER TABLE inventory_movements DROP CONSTRAINT inventory_movements_movement_type_check;
ALTER TABLE inventory_movements ADD CONSTRAINT inventory_movements_movement_type_check
 CHECK(movement_type IN ('opening','count_reconciliation','sale_consumption','wastage','breakage','transfer_out','transfer_in','purchase_receipt','manual_adjustment','sale_void_reversal','sale_refund_reversal'));
CREATE UNIQUE INDEX inventory_sale_reversal_once_uq
 ON inventory_movements(business_id,reference_id,branch_id,product_id,movement_type)
 WHERE movement_type IN ('sale_void_reversal','sale_refund_reversal') AND reference_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS inventory_sale_reversal_once_uq;
ALTER TABLE inventory_movements DROP CONSTRAINT inventory_movements_movement_type_check;
ALTER TABLE inventory_movements ADD CONSTRAINT inventory_movements_movement_type_check
 CHECK(movement_type IN ('opening','count_reconciliation','sale_consumption','wastage','breakage','transfer_out','transfer_in','purchase_receipt','manual_adjustment'));
