# Security model

1. Every business-owned row carries `business_id`.
2. Authenticated membership is the source of tenant authority. Browser-supplied tenant IDs are selectors at most, never proof of access.
3. Validate account status, business membership, branch membership and permission on each request.
4. Set PostgreSQL tenant context with `SET LOCAL` inside the same transaction used for tenant queries. Never use session-scoped tenant settings with pooled connections.
5. RLS is defence in depth; application authorization remains mandatory.
6. Audit sensitive operations. Audit records are append-only to application roles.
7. Manager PINs must be hashed, rate-limited, action-scoped and audited; never turn PIN verification into an unrestricted elevated session.
8. Do not place integration credentials or signing secrets in frontend environment variables.
9. Sessions must be opaque, random, stored hashed server-side, expiring and revocable.
10. Super-admin operations require separate explicit authorization and audit.

## Current implementation status (2026-11-23)

- Session tokens are random, opaque values; only SHA-256 digests are stored.
- Passwords use bcrypt. Registration rejects passwords above bcrypt's 72-byte input limit to prevent silent truncation semantics.
- Login attempts are atomically counted in PostgreSQL, with a 10-attempt / 15-minute window per normalized email.
- JSON endpoints reject non-JSON content types, unknown fields, oversized bodies and multiple JSON values.
- Cookie-authenticated state-changing requests require an exact Origin match with the configured web origin.
- Migration `003_runtime_role_and_rls.sql` creates a dedicated `mekuin_app` runtime role; migration `007_identity_and_business_rls.sql` completes RLS coverage for businesses, users and sessions.
- Compose now connects the API as `mekuin_app`; Goose continues to connect as the database bootstrap/migration owner.
- Registration uses a transaction-local bootstrap flag, then establishes user and business context before inserting the tenant-scoped branch and membership records.
- Workspace discovery uses a narrowly scoped `SECURITY DEFINER` function and a transaction-local authenticated `app.user_id`, because users may belong to more than one tenant.

**Remaining security work:** no live PostgreSQL integration or adversarial tenant-isolation test has run in this environment. The SECURITY DEFINER functions rely on the controlled migration owner having the ability to bypass forced RLS; the runtime role must never have BYPASSRLS or own application tables. The application still needs IP/device-aware login throttling, complete domain routes and permission enforcement, and production observability. Tenant isolation is improved in code but **not yet verified**.

## Tenant transaction boundary (Phase 1 increment)

`services/api/cmd/api/tenant.go` provides `withTenantTx`. It starts a transaction,
validates the authenticated user + business + branch against the security-definer
`resolve_branch_membership` function, and only then sets `app.user_id`,
`app.business_id`, and `app.branch_id` using transaction-local settings. All
branch-scoped repository work must run inside the callback. Do not set tenant
context on a pool connection or accept it as trusted client input.

The helper is infrastructure, not a substitute for calling `hasPermission` at
each protected use case. The role-permission matrix is intentionally conservative
and must be expanded alongside real use cases. `owner` is business-wide only;
it does not imply MEKU platform super-admin privileges.

The membership resolver checks active user, business, membership and branch
records. It is a narrowly scoped SECURITY DEFINER function with a fixed search
path and EXECUTE revoked from PUBLIC. Review its owner and grants during every
production database deployment.

## Selected tenant request context (Phase 1 increment)

Tenant-scoped routes must be mounted behind `requireTenant`. The middleware loads the selected business and branch from the authenticated server-side session, validates active membership, business status, branch status and branch assignment via `resolve_branch_membership`, then establishes transaction-local context. It does not trust tenant headers. The current `/api/v1/tenant/context` route is a diagnostic endpoint. Keep mutation handlers from writing a success response until their transaction has committed; the current middleware logs commit errors after the handler and is intended for read-only Phase 1 diagnostics until response buffering/commit semantics are added.

## Session-selected workspace (Phase 1)

The user selects a business and branch through `POST /api/v1/tenant/select`.
The API validates the selection with `resolve_branch_membership` and persists
both IDs on the authenticated session. Protected tenant routes do not accept
business or branch IDs as authority from request headers; `requireTenant` reads
the session selection and revalidates active user, business, membership and
branch on every request. Changing or revoking membership therefore invalidates
access on the next protected request.

`requireTenant` holds a transaction open for the handler and sets transaction-
local `app.user_id`, `app.business_id` and `app.branch_id` values. Tenant-aware
handlers must obtain the transaction with `tenantTxFromContext` and perform
all tenant SQL through it; using the pool directly bypasses the transaction's
RLS context. `requirePermission("permission.name")` is the backend permission
middleware for tenant-protected routes. It must be attached to each business
operation route; frontend role checks are never authorization.

The current Phase 1 API has only the tenant-context diagnostic route, not the
business operation routes. Consequently the permission middleware is unit-tested
but there are not yet sales, inventory, purchasing or reporting endpoints to
protect. Each future route must declare its permission explicitly.

## Identity and business RLS (migration 007)

`businesses`, `app_users` and `user_sessions` now have ENABLE + FORCE RLS. Ordinary app-user reads require matching `app.user_id`; ordinary business reads require matching `app.business_id`; session reads and updates require matching `app.user_id`. Login and session-token resolution use `lookup_auth_user` and `lookup_session`, narrow SECURITY DEFINER functions with fixed search paths and EXECUTE revoked from PUBLIC. Registration uses a transaction-local bootstrap policy only for the initial user/business creation transaction. The migration owner must be a tightly controlled migration principal capable of bypassing forced RLS; never grant BYPASSRLS to `mekuin_app`.

`db/tests/tenant_rls_smoke.sql` is a manual PostgreSQL smoke test. Run it with `psql` as the migration owner after all migrations. It creates fixtures inside a transaction, switches to `mekuin_app`, verifies tenant-limited reads, tests cross-tenant write denial, then rolls back. It has not been executed in this environment.


## Cookie issuance and origin validation

All `/api/v1` state-changing requests are checked against the exact configured `WEB_ORIGIN`, including registration and login before a session cookie is issued. Requests without an `Origin` header are rejected for state-changing API operations. Configure `SESSION_COOKIE_SECURE=true` whenever the application is served over HTTPS; the Compose default is secure, while `.env.example` opts into `false` only for local HTTP development.
