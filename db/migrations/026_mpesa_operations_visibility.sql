-- +goose Up
-- Supports branch-scoped operational queues without exposing callback payloads.
CREATE INDEX mpesa_attempt_branch_recent_idx
 ON mpesa_payment_attempts(business_id,branch_id,created_at DESC,id DESC);
CREATE INDEX mpesa_attempt_branch_review_idx
 ON mpesa_payment_attempts(business_id,branch_id,updated_at DESC)
 WHERE status IN ('created','accepted','callback_received','verification_pending','provider_confirmed','review_required');

-- +goose Down
DROP INDEX IF EXISTS mpesa_attempt_branch_review_idx;
DROP INDEX IF EXISTS mpesa_attempt_branch_recent_idx;
