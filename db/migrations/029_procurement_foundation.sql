-- +goose Up
-- Phase 4B foundation: tenant-safe procurement records and immutable receipts.
-- Receiving transaction/API logic is introduced in the next slice.
ALTER TABLE inventory_movements DROP CONSTRAINT inventory_movements_movement_type_check;
ALTER TABLE inventory_movements ADD CONSTRAINT inventory_movements_movement_type_check
 CHECK(movement_type IN ('opening','count_reconciliation','sale_consumption','wastage','breakage','transfer_out','transfer_in','purchase_receipt'));

CREATE TABLE purchase_orders (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), business_id uuid NOT NULL, branch_id uuid NOT NULL,
 supplier_id uuid NOT NULL, po_number text NOT NULL CHECK(length(trim(po_number)) BETWEEN 1 AND 80),
 status text NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','ordered','partially_received','received','cancelled')),
 notes text, created_by uuid NOT NULL, ordered_at timestamptz, received_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(business_id,id), UNIQUE(business_id,po_number),
 FOREIGN KEY(business_id,branch_id) REFERENCES branches(business_id,id),
 FOREIGN KEY(business_id,supplier_id) REFERENCES suppliers(business_id,id),
 FOREIGN KEY(business_id,created_by) REFERENCES memberships(business_id,user_id)
);
CREATE INDEX purchase_orders_branch_status_idx ON purchase_orders(business_id,branch_id,status,created_at DESC);
ALTER TABLE purchase_orders ENABLE ROW LEVEL SECURITY; ALTER TABLE purchase_orders FORCE ROW LEVEL SECURITY;
CREATE POLICY purchase_orders_tenant_policy ON purchase_orders USING(business_id=nullif(current_setting('app.business_id',true),'')::uuid) WITH CHECK(business_id=nullif(current_setting('app.business_id',true),'')::uuid);
REVOKE UPDATE,DELETE ON purchase_orders FROM mekuin_app;
GRANT SELECT,INSERT ON purchase_orders TO mekuin_app;

CREATE TABLE purchase_order_lines (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), business_id uuid NOT NULL, purchase_order_id uuid NOT NULL,
 product_id uuid NOT NULL, ordered_quantity numeric(20,8) NOT NULL CHECK(ordered_quantity>0),
 unit_cost_minor bigint NOT NULL CHECK(unit_cost_minor>=0), received_quantity numeric(20,8) NOT NULL DEFAULT 0 CHECK(received_quantity>=0 AND received_quantity<=ordered_quantity),
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(business_id,id), UNIQUE(business_id,purchase_order_id,product_id),
 FOREIGN KEY(business_id,purchase_order_id) REFERENCES purchase_orders(business_id,id),
 FOREIGN KEY(business_id,product_id) REFERENCES products(business_id,id)
);
ALTER TABLE purchase_order_lines ENABLE ROW LEVEL SECURITY; ALTER TABLE purchase_order_lines FORCE ROW LEVEL SECURITY;
CREATE POLICY purchase_order_lines_tenant_policy ON purchase_order_lines USING(business_id=nullif(current_setting('app.business_id',true),'')::uuid) WITH CHECK(business_id=nullif(current_setting('app.business_id',true),'')::uuid);
REVOKE UPDATE,DELETE ON purchase_order_lines FROM mekuin_app;
GRANT SELECT,INSERT ON purchase_order_lines TO mekuin_app;

CREATE TABLE purchase_receipts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), business_id uuid NOT NULL, branch_id uuid NOT NULL,
 purchase_order_id uuid NOT NULL, received_by uuid NOT NULL, note text,
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(business_id,id),
 FOREIGN KEY(business_id,branch_id) REFERENCES branches(business_id,id),
 FOREIGN KEY(business_id,purchase_order_id) REFERENCES purchase_orders(business_id,id),
 FOREIGN KEY(business_id,received_by) REFERENCES memberships(business_id,user_id)
);
CREATE INDEX purchase_receipts_po_idx ON purchase_receipts(business_id,purchase_order_id,created_at);
ALTER TABLE purchase_receipts ENABLE ROW LEVEL SECURITY; ALTER TABLE purchase_receipts FORCE ROW LEVEL SECURITY;
CREATE POLICY purchase_receipts_tenant_policy ON purchase_receipts USING(business_id=nullif(current_setting('app.business_id',true),'')::uuid) WITH CHECK(business_id=nullif(current_setting('app.business_id',true),'')::uuid);
REVOKE UPDATE,DELETE ON purchase_receipts FROM mekuin_app;
GRANT SELECT,INSERT ON purchase_receipts TO mekuin_app;

CREATE TABLE purchase_receipt_lines (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), business_id uuid NOT NULL, receipt_id uuid NOT NULL,
 purchase_order_line_id uuid NOT NULL, product_id uuid NOT NULL, received_quantity numeric(20,8) NOT NULL CHECK(received_quantity>0),
 unit_cost_minor bigint NOT NULL CHECK(unit_cost_minor>=0), movement_id uuid NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(business_id,id), UNIQUE(business_id,movement_id),
 FOREIGN KEY(business_id,receipt_id) REFERENCES purchase_receipts(business_id,id),
 FOREIGN KEY(business_id,purchase_order_line_id) REFERENCES purchase_order_lines(business_id,id),
 FOREIGN KEY(business_id,product_id) REFERENCES products(business_id,id)
);
ALTER TABLE purchase_receipt_lines ENABLE ROW LEVEL SECURITY; ALTER TABLE purchase_receipt_lines FORCE ROW LEVEL SECURITY;
CREATE POLICY purchase_receipt_lines_tenant_policy ON purchase_receipt_lines USING(business_id=nullif(current_setting('app.business_id',true),'')::uuid) WITH CHECK(business_id=nullif(current_setting('app.business_id',true),'')::uuid);
REVOKE UPDATE,DELETE ON purchase_receipt_lines FROM mekuin_app;
GRANT SELECT,INSERT ON purchase_receipt_lines TO mekuin_app;

-- The API records movement rows as append-only ledger entries; this migration
-- establishes the type and receipt record linkage but intentionally grants no
-- ledger mutation privilege and no direct UPDATE privilege on procurement rows.

-- +goose Down
DROP TABLE IF EXISTS purchase_receipt_lines;
DROP TABLE IF EXISTS purchase_receipts;
DROP TABLE IF EXISTS purchase_order_lines;
DROP TABLE IF EXISTS purchase_orders;
ALTER TABLE inventory_movements DROP CONSTRAINT inventory_movements_movement_type_check;
ALTER TABLE inventory_movements ADD CONSTRAINT inventory_movements_movement_type_check
 CHECK(movement_type IN ('opening','count_reconciliation','sale_consumption','wastage','breakage','transfer_out','transfer_in'));
