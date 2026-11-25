# Phase 8A — Platform administration console

## Scope

Adds a restricted Next.js platform console at `/platform` over the existing Phase 7 API. The API remains the authorization boundary: every request is authenticated and revalidated as an active database-backed super-admin by `requireSuperAdmin`. The page does not treat client-side visibility as authorization.

## Delivered

- Business directory with active/suspended filters and bounded pagination.
- User directory with active/suspended filters and bounded pagination.
- Platform audit history with business/user entity filtering and bounded pagination.
- Business and user status actions with mandatory 8–500 character operator reasons.
- Confirmation dialog explains session revocation and active-owner restrictions for user suspension.
- Status changes are submitted only to the existing audited API endpoints; the UI does not mutate tenant data or platform roles.
- Unauthorized users receive the API's access denial; unauthenticated users are redirected to sign-in.
- No support impersonation, cross-tenant operational browsing, deletion, billing or super-admin promotion is introduced.

## Routes used

- `GET /api/v1/platform/businesses`
- `PATCH /api/v1/platform/businesses/{businessID}/status`
- `GET /api/v1/platform/users`
- `PATCH /api/v1/platform/users/{userID}/status`
- `GET /api/v1/platform/audit`

## Verification and release gates

- Static TSX transpilation and archive integrity are checked for this source increment.
- Run the complete Next.js lint/typecheck/build and Go 1.24.13 test/race/vet/build suites.
- Apply migrations through 042 and verify audit table grants on PostgreSQL 17.
- Test `/platform` with an active super-admin, ordinary user, suspended user and expired session.
- Verify suspension/reactivation actions, owner-protection conflict, session revocation and audit event creation in authenticated staging.

This is an API-backed administrative interface, not production acceptance. Platform access must remain restricted to trusted super-admin accounts.
