# Phase 1 acceptance and operations checklist

This document distinguishes implemented controls from verification that still requires a running PostgreSQL/Docker environment.

## Runtime database role provisioning

The Goose migration creates `mekuin_app` without embedding a password. Compose then runs the `runtime-role` one-shot service, which applies `MEKUIN_APP_PASSWORD` through `psql` variable quoting before the API starts. Keep the migration connection (`POSTGRES_USER`) separate from the API connection (`mekuin_app`). Do not use the example secrets in staging or production. Rotate the database owner and runtime passwords independently.

The Goose image is pinned to `v3.24.3`; update it deliberately and test migrations before deployment.

## Authorization lifecycle

- Authentication checks the user account status and session expiry/revocation on every request.
- Workspace discovery returns only active memberships and active branches, and excludes suspended businesses.
- Workspace selection is stored on the server-side session and validates active user, business, membership and branch.
- Tenant-protected requests load the selected workspace from that session and revalidate the membership and all active statuses on each request.
- `requirePermission` is the backend authorization middleware. Apply it to every future operation route; frontend role/permission visibility is not authorization.
- Tenant SQL must use `tenantTxFromContext` and the transaction established by `requireTenant`. Do not use the pool directly inside tenant handlers, because transaction-local RLS settings would not apply to another connection.

## Audit immutability

Migration 006 adds a database trigger rejecting UPDATE/DELETE on `audit_logs` and removes those privileges from the runtime role. Audit inserts remain permitted subject to tenant RLS. Privileged database owners can still alter schema/triggers; production database administration must be tightly controlled and separately audited.

## Mandatory pre-release verification

Run these from the repository root with Docker available:

1. `cp .env.example .env`, replace both passwords with independent URL-safe random secrets (for example, base64url output) because Compose currently embeds them in PostgreSQL connection URLs, then `docker compose up --build`.
2. Confirm `db`, `migrate`, and `runtime-role` complete successfully before `api` starts.
3. Run `docker compose exec db psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "\\du+ mekuin_app"` and verify the role is LOGIN, NOSUPERUSER, NOCREATEDB, NOCREATEROLE, NOINHERIT.
4. Confirm the API connects using `mekuin_app`, not the owner account.
5. Register owner A and owner B in separate businesses; verify each sees only their own workspace.
6. Attempt selection using another user's business/branch UUID; expect 403 and no session selection change.
7. Select a workspace, suspend its business, then call `/api/v1/tenant/context`; expect denial.
8. Reactivate the business, suspend the membership, and repeat; expect denial.
9. Revoke the session and repeat; expect 401.
10. Verify role middleware: cashier denied `sales.void`; manager allowed `sales.void`; stock controller denied `sales.refund`.
11. Attempt UPDATE and DELETE on `audit_logs` as `mekuin_app`; both must fail.
12. Verify tenant RLS using two business contexts and attempt direct cross-tenant reads/writes on businesses, branches, memberships, membership_branches and audit_logs.
13. As `mekuin_app`, verify own-user/session reads succeed only with matching `app.user_id`, and other users/sessions remain invisible.
14. Verify `lookup_auth_user` returns only the requested login identity and `lookup_session` rejects revoked/expired tokens.
15. Verify cookie-authenticated POST requests without the exact configured Origin are rejected; matching Origin succeeds.

## Known Phase 1 security gaps

- RLS policies now cover `businesses`, `app_users`, `user_sessions`, `branches`, `memberships`, `membership_branches` and `audit_logs`; validate all policies and privileged functions against a real PostgreSQL instance before treating them as verified.
- Permission middleware exists and is unit-testable, but there are not yet domain operation routes to protect. Each new route must declare its required permission.
- There are no PostgreSQL integration tests or complete browser-to-database end-to-end tests in this starter archive.
- Login throttling is currently keyed by normalized email, not client IP/device; add distributed edge/API rate limiting before public launch.
- Cookie/CORS/CSRF settings must be configured for the actual production domain topology. Do not assume local `SameSite=Lax` settings are suitable for unrelated frontend/API sites.

Phase 1 must not be marked production-ready until the mandatory checks pass and the known gaps are either closed or explicitly accepted with compensating controls.

## Identity and business RLS (migration 007)

Migration 007 enables and forces RLS on `businesses`, `app_users`, and `user_sessions` in addition to the tenant tables covered by earlier migrations. The runtime role's ordinary reads are constrained by transaction-local `app.business_id` or `app.user_id` settings. Login and opaque session-token resolution use two narrow SECURITY DEFINER functions (`lookup_auth_user`, `lookup_session`) so unauthenticated requests do not receive broad table SELECT privileges. The registration transaction uses a short-lived `app.registration=true` context to bootstrap the first user and business; this is a bootstrap exception, not a general tenant operation.

The migration owner must own these SECURITY DEFINER functions and must be a tightly controlled migration principal with the ability to bypass forced RLS (the local Compose database owner is PostgreSQL's bootstrap superuser). Do not run migrations as the runtime role. The application role is not a superuser and must never own application tables.

The API sets `app.user_id` before session creation, own-session reads/updates and own-profile reads. Tenant request middleware sets user/business/branch context on the same transaction used by tenant SQL. Cross-tenant SQL must never be issued through the pool from a tenant handler.

## Browser mutation protection

Cookie-authenticated mutation routes require an exact `Origin` match with `WEB_ORIGIN`; GET/HEAD/OPTIONS are exempt. Configure `WEB_ORIGIN` to the exact deployed browser origin. Keep HTTPS and `SESSION_COOKIE_SECURE=true` in production. If the frontend and API are deployed on unrelated sites, reassess cookie SameSite behavior and use a deliberate CSRF-token strategy rather than weakening origin checks.

## Current implementation boundaries

- `lookup_auth_user` and `lookup_session` are privileged lookup functions. Keep their return values minimal and never add password reset tokens or unrelated personal data to them.
- RLS context variables are transaction-local and protect against accidental unscoped SQL. They are not a boundary against arbitrary SQL execution by a fully compromised runtime role, because that role can call `set_config`; parameterized SQL, least privilege, and prevention of injection remain mandatory.
- The registration bootstrap policy permits registration-scoped SELECT/INSERT while `app.registration=true`. Keep this transaction limited to first-user/business/branch/membership creation and do not reuse the flag for normal requests.
- The API currently exposes no catalog/POS domain mutation routes. Permission middleware must be attached to every future operation endpoint before it is exposed.
- A real PostgreSQL test run is still mandatory to validate migration ownership, SECURITY DEFINER execution, forced RLS, grants, and the full browser/session flow. Static inspection is not a substitute.


### Cookie and origin deployment check

- Set `WEB_ORIGIN` to the single canonical frontend origin (scheme and host, no path).
- Set `SESSION_COOKIE_SECURE=true` for all HTTPS deployments.
- Verify login and registration reject missing or foreign Origin headers, and accept the configured frontend origin.
- Do not use the local `.env.example` cookie setting in production.

## Phase 2A schema increment (migration 008)

Migration `008_catalog_configuration.sql` adds tenant-scoped staff profiles, units of measure, explicit unit conversions, product categories, unit/liquid products, suppliers, recipes and recipe ingredients, and branch-specific bar tables. It also seeds a read-only role-permission matrix matching the current Go authorization map. Catalog tables use forced RLS keyed to transaction-local `app.business_id`; relationships use composite `(business_id, id)` foreign keys to prevent cross-tenant references.

Product costs and sale prices are integer minor units. Quantities and conversion factors use exact `NUMERIC`; conversion factors and recipe quantities must be positive. Liquid products require positive bottle and serving sizes, with serving size no greater than bottle size. This schema does not decide stock valuation, tax treatment, negative-stock policy, or recipe costing/rounding policy. The API must hash manager PINs before persistence; plaintext PINs must never be stored or logged.

Run `psql "$MIGRATION_DATABASE_URL" -v ON_ERROR_STOP=1 -f db/tests/catalog_rls_smoke.sql` after `goose up` to exercise tenant reads/writes, positive conversion rules, liquid-size validation, duplicate SKU protection and runtime permission-matrix immutability. This smoke test has not been executed against PostgreSQL in this environment; passing Go unit tests alone does not validate migration behavior.
