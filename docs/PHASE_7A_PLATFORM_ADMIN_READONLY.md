# Phase 7A — Platform administration foundation (read-only)

## Scope delivered

- Added an authenticated platform route group at `GET /api/v1/platform/businesses`.
- Added a server-side `requireSuperAdmin` guard. The guard re-reads `app_users.platform_role` and `account_status` from PostgreSQL for every request; it does not trust browser state, tenant role, request headers, or a client-supplied role.
- Added bounded pagination (`limit` 1–100, default 50; `offset` 0–1,000,000) and an optional `active`/`suspended` status filter.
- Returns a deliberately minimal business summary: ID, name, currency, account status, creation timestamp, branch count and active membership count.
- Does not expose user email addresses, financial records, transaction data, or tenant operational records.
- Added unit coverage for the active super-admin predicate.

## Security and bootstrap requirements

- The `platform_role` column already exists in the foundation schema; this increment does not add a migration.
- No public API is provided to grant or revoke `super_admin`.
- Initial super-admin assignment must be performed through a separately controlled, audited database-owner procedure. Never promote an account through a browser or an unauthenticated endpoint.
- Suspended users and ordinary users are denied even if a stale browser session remains present.
- The endpoint is platform-level and intentionally does not enter a tenant transaction or change tenant RLS context.

## Not included

Business suspension/reactivation, user suspension, support impersonation, tenant data access, destructive administration, deletion/retention workflows, super-admin UI, and admin audit-event storage are not enabled in this increment. These require explicit lifecycle and retention policies.

## Verification

- `gofmt` applied to changed Go files.
- Go test execution was attempted, but this environment has Go 1.23.2 while the repository pins Go 1.24.13; downloading the pinned toolchain was blocked by network DNS. No Go test pass is claimed.
- PostgreSQL integration, API authorization tests, and browser/UI acceptance remain external checks.
