-- +goose Up
ALTER TABLE user_sessions
  ADD COLUMN selected_business_id uuid,
  ADD COLUMN selected_branch_id uuid,
  ADD CONSTRAINT user_sessions_selected_workspace_pair CHECK (
    (selected_business_id IS NULL AND selected_branch_id IS NULL) OR
    (selected_business_id IS NOT NULL AND selected_branch_id IS NOT NULL)
  );
CREATE INDEX user_sessions_selected_workspace_idx
  ON user_sessions(selected_business_id, selected_branch_id)
  WHERE selected_business_id IS NOT NULL;
GRANT UPDATE (selected_business_id, selected_branch_id) ON user_sessions TO mekuin_app;

-- +goose Down
DROP INDEX IF EXISTS user_sessions_selected_workspace_idx;
ALTER TABLE user_sessions DROP CONSTRAINT IF EXISTS user_sessions_selected_workspace_pair;
ALTER TABLE user_sessions DROP COLUMN IF EXISTS selected_branch_id;
ALTER TABLE user_sessions DROP COLUMN IF EXISTS selected_business_id;
