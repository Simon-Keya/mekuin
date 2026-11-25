-- +goose Up
ALTER TABLE staff_profiles
  ADD COLUMN manager_pin_failed_attempts integer NOT NULL DEFAULT 0 CHECK (manager_pin_failed_attempts >= 0),
  ADD COLUMN manager_pin_window_started_at timestamptz,
  ADD COLUMN manager_pin_locked_until timestamptz;

CREATE TABLE manager_action_approvals (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  business_id uuid NOT NULL REFERENCES businesses(id),
  branch_id uuid NOT NULL,
  requester_user_id uuid NOT NULL REFERENCES app_users(id),
  approver_membership_id uuid NOT NULL,
  action text NOT NULL CHECK (action IN ('discount.approve','sales.void','sales.refund','inventory.count.approve')),
  target_id uuid,
  token_hash bytea NOT NULL UNIQUE,
  expires_at timestamptz NOT NULL,
  consumed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (business_id, branch_id) REFERENCES branches(business_id,id),
  FOREIGN KEY (business_id, approver_membership_id) REFERENCES memberships(business_id,id)
);
ALTER TABLE manager_action_approvals ENABLE ROW LEVEL SECURITY;
ALTER TABLE manager_action_approvals FORCE ROW LEVEL SECURITY;
CREATE POLICY manager_action_approvals_tenant_policy ON manager_action_approvals
  USING (business_id = nullif(current_setting('app.business_id', true), '')::uuid)
  WITH CHECK (business_id = nullif(current_setting('app.business_id', true), '')::uuid);
GRANT SELECT, INSERT, UPDATE ON manager_action_approvals TO mekuin_app;
CREATE INDEX manager_action_approvals_expiry_idx ON manager_action_approvals(business_id, expires_at) WHERE consumed_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS manager_action_approvals;
ALTER TABLE staff_profiles DROP COLUMN IF EXISTS manager_pin_locked_until;
ALTER TABLE staff_profiles DROP COLUMN IF EXISTS manager_pin_window_started_at;
ALTER TABLE staff_profiles DROP COLUMN IF EXISTS manager_pin_failed_attempts;
