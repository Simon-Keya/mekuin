-- +goose Up
-- Multiple immutable sale_payments rows represent individual tender allocations.
-- This index supports branch-scoped tender reconciliation without changing history.
CREATE INDEX sale_payments_branch_method_created_idx
 ON sale_payments(business_id,branch_id,method,created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS sale_payments_branch_method_created_idx;
