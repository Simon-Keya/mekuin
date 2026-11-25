-- +goose Up
-- Runtime code may create approvals and consume them once. It must not be able
-- to rewrite their identity, scope, action, token digest, expiry or audit time.
REVOKE SELECT, INSERT, UPDATE ON manager_action_approvals FROM mekuin_app;
GRANT SELECT (id, business_id, branch_id, requester_user_id, action, target_id, token_hash, expires_at, consumed_at)
  ON manager_action_approvals TO mekuin_app;
GRANT INSERT (business_id, branch_id, requester_user_id, approver_membership_id, action, target_id, token_hash, expires_at)
  ON manager_action_approvals TO mekuin_app;
GRANT UPDATE (consumed_at) ON manager_action_approvals TO mekuin_app;

-- +goose Down
REVOKE SELECT, INSERT, UPDATE ON manager_action_approvals FROM mekuin_app;
GRANT SELECT, INSERT, UPDATE ON manager_action_approvals TO mekuin_app;
