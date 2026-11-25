# Phase 2R acceptance work

This increment adds:

- `POST /api/v1/staff/memberships`: attach an existing active account to the selected business, assign active branches atomically, and audit the operation. The account must register first; invitation email and acceptance are not implemented.
- `011_staff_membership_lookup.sql`: a narrow SECURITY DEFINER lookup returning only the UUID for an exact active-account email match.
- Expanded `db/tests/api_catalog_smoke.sh`: catalog HTTP flow, cross-tenant workspace denial, staff membership validation and provisioning, profile/role/branch/PIN lifecycle, manager PIN approval success and five-failure lockout, directional conversion, and recipe cost/margin assertions.
- OpenAPI 3.1 v0.4.0 with cookie auth, common error response, and schemas for key Phase 2 request/response contracts.

## Verification in this environment

Passed: `gofmt`, shell syntax check (`sh -n`), OpenAPI JSON parsing, CI YAML parsing, and archive integrity.

Not executed: Go tests, `go vet`, production build, Goose migrations, PostgreSQL 17 RLS tests, and API HTTP smoke tests. Go 1.24.13 could not be downloaded because outbound DNS/network access is unavailable; no PostgreSQL client or container runtime is installed. These checks are wired into GitHub Actions but must complete successfully there before acceptance.

## Still open

- Email invitations and invitation acceptance (current provisioning requires an existing active account).
- A real sensitive sales/inventory HTTP operation consuming manager approvals inside its own transaction (no such handler exists yet).
- Full OpenAPI schemas for every maintenance endpoint and all success/error payloads.
- Hosted CI green run and production environment smoke tests.

## Phase 2T — OpenAPI contract enforcement

- Added `openapi_test.go` to make the published OpenAPI document executable contract coverage: required Phase 2 paths/methods, authenticated tenant operations, request-body schema references, and shared structured error schemas are checked by `go test`.
- Normalized all documented 4xx/5xx/default responses to the common `Error` schema and ensured every operation declares a default structured error response.
- The API smoke suite from Phase 2S remains the executable HTTP coverage for cross-tenant workspace selection, staff membership provisioning, branch scoping, manager PIN lockout, unit conversion and recipe costing.
- This is source-level and CI coverage, not proof of a green CI run. Local `go test ./...` was attempted but the Go 1.24.13 toolchain download failed because the environment cannot resolve `proxy.golang.org`. PostgreSQL 17 is likewise not available locally; CI remains the execution environment for those checks.
- Manager approvals remain authorization primitives until an actual sales/refund/inventory operation is introduced. Do not add a standalone token-consumption endpoint: approval consumption must be in the same database transaction as the sensitive business operation.
