-- +goose Up
-- Provider query confirmation is deliberately not the same as settlement.
ALTER TABLE mpesa_payment_attempts DROP CONSTRAINT IF EXISTS mpesa_payment_attempts_status_check;
ALTER TABLE mpesa_payment_attempts ADD CONSTRAINT mpesa_payment_attempts_status_check
 CHECK (status IN ('created','accepted','callback_received','verification_pending','provider_confirmed','verified','failed','expired','review_required'));
ALTER TABLE mpesa_payment_attempts ADD COLUMN provider_queried_at timestamptz;
ALTER TABLE mpesa_payment_attempts ADD COLUMN provider_query_result_code text;
ALTER TABLE mpesa_payment_attempts ADD COLUMN provider_query_description text;
ALTER TABLE mpesa_payment_attempts ADD COLUMN callback_metadata_matched_at timestamptz;
CREATE INDEX mpesa_attempt_provider_confirmed_idx ON mpesa_payment_attempts(business_id,branch_id,created_at DESC) WHERE status='provider_confirmed';
GRANT UPDATE (status,provider_result_code,provider_result_description,mpesa_receipt_number,transaction_at,failed_at,provider_queried_at,provider_query_result_code,provider_query_description,callback_metadata_matched_at,updated_at) ON mpesa_payment_attempts TO mekuin_app;

-- +goose Down
DROP INDEX IF EXISTS mpesa_attempt_provider_confirmed_idx;
ALTER TABLE mpesa_payment_attempts DROP COLUMN IF EXISTS callback_metadata_matched_at;
ALTER TABLE mpesa_payment_attempts DROP COLUMN IF EXISTS provider_query_description;
ALTER TABLE mpesa_payment_attempts DROP COLUMN IF EXISTS provider_query_result_code;
ALTER TABLE mpesa_payment_attempts DROP COLUMN IF EXISTS provider_queried_at;
ALTER TABLE mpesa_payment_attempts DROP CONSTRAINT IF EXISTS mpesa_payment_attempts_status_check;
ALTER TABLE mpesa_payment_attempts ADD CONSTRAINT mpesa_payment_attempts_status_check
 CHECK (status IN ('created','accepted','callback_received','verification_pending','verified','failed','expired','review_required'));
