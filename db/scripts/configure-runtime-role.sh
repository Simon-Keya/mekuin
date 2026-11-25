#!/bin/sh
set -eu
: "${MIGRATION_DATABASE_URL:?MIGRATION_DATABASE_URL is required}"
: "${MEKUIN_APP_PASSWORD:?MEKUIN_APP_PASSWORD is required}"
# psql variable interpolation safely quotes the password as a SQL literal.
psql "$MIGRATION_DATABASE_URL" -v ON_ERROR_STOP=1 -v app_password="$MEKUIN_APP_PASSWORD" <<'SQL'
ALTER ROLE mekuin_app PASSWORD :'app_password';
SQL
