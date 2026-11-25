# Phase 8 implementation report

**Baseline:** Phase 8A platform console (v86)  
**Delivery:** Phase 8 consolidated source increment (v87)

## Implemented in this increment

- Added super-admin-only `POST /api/v1/platform/businesses`.
- Provisions a new business and initial branch for an existing active account.
- Creates the initial owner membership and branch assignment atomically.
- Creates the standard default expense categories in the same transaction.
- Writes a platform audit event in the same transaction; a failed audit write rolls back provisioning.
- Resolves owner email through the existing restricted authentication lookup function, then locks the owner account under its own RLS identity and confirms it remains active.
- Adds a console provisioning dialog with validation, mandatory reason, error feedback and refreshed directory state.
- Adds unit coverage for provisioning input validation.
- Documents deferred invitation, billing and ownership-transfer scope.

## Checks actually run

- `gofmt` on changed Go files: passed.
- TypeScript transpilation syntax check for the platform page: passed.
- ZIP integrity (`unzip -t`): passed.

## Checks not run

- Go tests: unavailable because the pinned Go 1.24.13 toolchain could not be downloaded in this environment.
- PostgreSQL 17 migration/RLS test: not run.
- Next.js full type-check/production build: not run.
- Authenticated browser acceptance: not run.

## Deployment acceptance notes

Run the provisioning test using the restricted `mekuin_app` runtime role. Verify that a non-super-admin receives 403, an unknown or suspended owner is rejected, all created rows commit together, and forced RLS does not permit cross-tenant reads. Confirm the owner sees the new workspace after signing in. Do not use a database owner connection as a substitute for the runtime-role test.
