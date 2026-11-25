-- +goose Up
-- A refund approval must authorize the exact amount, not merely the sale.
ALTER TABLE manager_action_approvals
  ADD COLUMN target_amount_minor bigint CHECK(target_amount_minor IS NULL OR target_amount_minor > 0);
REVOKE UPDATE ON manager_action_approvals FROM mekuin_app;
GRANT SELECT(target_amount_minor) ON manager_action_approvals TO mekuin_app;
GRANT INSERT(target_amount_minor) ON manager_action_approvals TO mekuin_app;
GRANT UPDATE(consumed_at) ON manager_action_approvals TO mekuin_app;

-- +goose Down
REVOKE UPDATE(consumed_at) ON manager_action_approvals FROM mekuin_app;
ALTER TABLE manager_action_approvals DROP COLUMN IF EXISTS target_amount_minor;
