# Phase 7 — Platform Administration Foundation and Closeout

## Scope delivered in this source increment

### 7A — Platform administration foundation
- Database-backed `super_admin` + active-account authorization is checked on every platform request.
- `GET /api/v1/platform/businesses` provides paginated, minimal business metadata and status filtering.
- No client-provided role or tenant membership is accepted as platform authority.

### 7B — Business lifecycle controls
- `PATCH /api/v1/platform/businesses/{businessID}/status` supports `active` and `suspended`.
- A reason (8–500 characters) is mandatory.
- Status change and platform audit event are committed in one transaction.
- Suspension preserves tenant data and memberships; the existing `resolve_branch_membership` function rejects access while the business is suspended.

### 7C — User lifecycle controls
- `GET /api/v1/platform/users` provides paginated user directory metadata for platform support.
- `PATCH /api/v1/platform/users/{userID}/status` supports activation and suspension with a mandatory reason.
- Suspension revokes all existing sessions in the same transaction.
- Self-suspension is rejected.
- A user with any active owner membership cannot be suspended. Transfer ownership through a tenant-governed workflow first; this API does not rewrite ownership.

### 7D — Platform audit trail
- `platform_audit_logs` records actor, action, entity, reason, metadata and timestamp.
- Lifecycle state changes and audit events are atomic.
- Runtime role receives SELECT/INSERT only; PUBLIC is explicitly denied UPDATE/DELETE.
- `GET /api/v1/platform/audit` returns the newest platform events.

## API request examples

```http
PATCH /api/v1/platform/businesses/{businessID}/status
Content-Type: application/json

{"status":"suspended","reason":"Customer requested temporary service suspension"}
```

```http
PATCH /api/v1/platform/users/{userID}/status
Content-Type: application/json

{"status":"suspended","reason":"Confirmed account compromise investigation"}
```

All platform endpoints require an authenticated active super-admin. State-changing calls also pass the application's same-origin middleware.

## Explicitly out of scope
- Creating or promoting super-admin accounts through an API.
- Editing tenant financial or operational records from platform administration.
- Impersonating tenant users or bypassing tenant authorization.
- Deleting businesses/users or cascading deletion of operational history.
- Ownership transfer; this must be a tenant-authorized, audited workflow.
- Billing, subscription entitlements, plan enforcement and payment collection.
- Support impersonation, remote access and cross-tenant data browsing.

## Release gates

This is an implementation closeout, not production acceptance. Before release:
1. Apply migrations 001–041 on PostgreSQL 17 in a disposable database and verify Goose up/down behaviour.
2. Verify `mekuin_app` can SELECT/INSERT platform audit events but cannot UPDATE/DELETE them.
3. Integration-test unauthenticated, ordinary-user, suspended-super-admin and active-super-admin access.
4. Test business suspension blocks `resolve_branch_membership` and reactivation restores eligible access without data loss.
5. Test user suspension revokes every session and blocks subsequent authentication; test reactivation does not resurrect revoked sessions.
6. Test that active owners are protected and that a failed audit insert rolls back the corresponding status change.
7. Run Go 1.24.13 tests, race, vet and build; Next.js lint/typecheck/build; then authenticated staging acceptance.

## Known limitation
The platform user directory contains email addresses because support needs account identification. Access is restricted to active super-admins. Add pagination, access monitoring and an approved support-data policy before exposing this UI broadly.

### 7E — Directory and audit query hardening (v85)
- User directory supports bounded `limit`/`offset` pagination and active/suspended filtering.
- Audit directory supports bounded `limit`/`offset` pagination and business/user entity filtering.
- Invalid filters and pagination are rejected before database queries.
- Added shared pagination parsing and unit coverage for boundary and malformed values.
- Added migration 042 to explicitly revoke UPDATE/DELETE/TRUNCATE/REFERENCES/TRIGGER from PUBLIC and `mekuin_app` on platform audit history, while retaining SELECT/INSERT for the application runtime.

Additional API examples:
```http
GET /api/v1/platform/users?status=suspended&limit=50&offset=0
GET /api/v1/platform/audit?entity_type=business&limit=50&offset=50
```

### 7E verification note
Go formatting was run. A root-level `go test ./...` invocation was invalid because the repository uses a nested module; run tests from `services/api`. PostgreSQL migration/privilege verification and authenticated endpoint tests remain release gates. No frontend platform administration console is included in this source increment.
