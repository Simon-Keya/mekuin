-- +goose Up
ALTER TABLE inventory_counts DROP CONSTRAINT inventory_counts_status_check;
ALTER TABLE inventory_counts ADD CONSTRAINT inventory_counts_status_check CHECK(status IN ('draft','submitted','reconciled'));
ALTER TABLE inventory_counts ADD COLUMN submitted_by uuid, ADD COLUMN submitted_at timestamptz;
ALTER TABLE inventory_counts ADD CONSTRAINT inventory_counts_submitted_by_fk FOREIGN KEY(business_id,submitted_by) REFERENCES memberships(business_id,user_id);
DROP INDEX inventory_counts_one_draft_branch_uq;
CREATE UNIQUE INDEX inventory_counts_one_open_branch_uq ON inventory_counts(business_id,branch_id) WHERE status IN ('draft','submitted');
GRANT UPDATE(counted_quantity) ON inventory_count_lines TO mekuin_app;
GRANT UPDATE(status,submitted_by,submitted_at) ON inventory_counts TO mekuin_app;

-- +goose Down
DROP INDEX IF EXISTS inventory_counts_one_open_branch_uq;
CREATE UNIQUE INDEX inventory_counts_one_draft_branch_uq ON inventory_counts(business_id,branch_id) WHERE status='draft';
ALTER TABLE inventory_counts DROP CONSTRAINT IF EXISTS inventory_counts_submitted_by_fk;
ALTER TABLE inventory_counts DROP COLUMN IF EXISTS submitted_at,DROP COLUMN IF EXISTS submitted_by;
ALTER TABLE inventory_counts DROP CONSTRAINT inventory_counts_status_check;
ALTER TABLE inventory_counts ADD CONSTRAINT inventory_counts_status_check CHECK(status IN ('draft','reconciled'));
