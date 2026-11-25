-- +goose Up
CREATE TABLE auth_login_limits (
  email_key text PRIMARY KEY,
  attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  window_started_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX auth_login_limits_window_idx ON auth_login_limits(window_started_at);

-- +goose Down
DROP TABLE IF EXISTS auth_login_limits;
