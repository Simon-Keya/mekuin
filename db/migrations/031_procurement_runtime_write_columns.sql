-- +goose Up
-- Keep procurement rows append-only except for narrowly scoped lifecycle counters.
GRANT UPDATE(status, ordered_at, received_at, updated_at) ON purchase_orders TO mekuin_app;
GRANT UPDATE(received_quantity) ON purchase_order_lines TO mekuin_app;
GRANT SELECT, INSERT ON purchase_orders, purchase_order_lines, purchase_receipts, purchase_receipt_lines TO mekuin_app;

-- +goose Down
REVOKE UPDATE(status, ordered_at, received_at, updated_at) ON purchase_orders FROM mekuin_app;
REVOKE UPDATE(received_quantity) ON purchase_order_lines FROM mekuin_app;
