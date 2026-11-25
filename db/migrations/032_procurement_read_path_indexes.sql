-- +goose Up
-- Support the purchase-order detail endpoint's receipt-history lookup and
-- receipt-line expansion without changing ledger or lifecycle semantics.
CREATE INDEX purchase_receipts_branch_history_idx
  ON purchase_receipts(business_id, branch_id, purchase_order_id, created_at DESC, id);

CREATE INDEX purchase_receipt_lines_receipt_history_idx
  ON purchase_receipt_lines(business_id, receipt_id, created_at, id);

-- +goose Down
DROP INDEX IF EXISTS purchase_receipt_lines_receipt_history_idx;
DROP INDEX IF EXISTS purchase_receipts_branch_history_idx;
