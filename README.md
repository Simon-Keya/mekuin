# MEKU-In

MEKU-In is a multi-tenant operations platform for Kenyan bars, lounges and nightclubs, built by MEKU SOLUTIONS.

## Stack
- Frontend: Next.js App Router, React, TypeScript, Tailwind.
- Backend: Go modular monolith using Chi, pgx and PostgreSQL.
- Database migrations: Goose.
- Local orchestration: Docker Compose.

## Run locally
Requirements: Docker Desktop, Node.js 22+, and Go 1.24.13. The Go toolchain is pinned consistently in `.go-version`, `.tool-versions`, `.mise.toml`, the Dev Container, CI and API Docker build.

Recommended local toolchain setup with [mise](https://mise.jdx.dev/):

```bash
mise install
go version  # should report go1.24.13
node --version
```

For a direct installation, run `scripts/install-go-1.24.13.ps1` in PowerShell or `sh scripts/install-go-1.24.13.sh` on Linux/macOS. Both scripts verify the official archive SHA-256 before installing. You can also use `mise install` or the included Dev Container. A repository cannot replace the host machine's Go binary without an installation step.

```bash
cp .env.example .env
docker compose up --build
```

Run backend checks from the repository root:

```bash
make test
make vet
make build
make fmt-check
```

CI installs Go 1.24.13 and runs Go tests, vet, build, PostgreSQL migrations and the tenant RLS smoke test. See [`docs/GO_TOOLCHAIN.md`](docs/GO_TOOLCHAIN.md) for installation and dependency-lock instructions.

Open http://localhost:3000. API health: http://localhost:8080/health/live.

The first-run flow supports owner registration, business and first-branch creation, login, session revocation and workspace loading. Use a unique email and a password of at least 12 characters.

## Current implementation boundary
The repository contains implemented increments across the foundation, catalog/configuration, POS and checkout, M-Pesa/Daraja payment lifecycle, inventory and procurement, supplier payables, cash shifts, expenses, operational alerts/audit, reporting/exports, offline POS order drafts and synchronization, and platform administration/onboarding. The phase documents under `docs/` describe each increment and its boundaries. This is still a work-in-progress release, not a claim of production readiness: full PostgreSQL 17/RLS execution, pinned Go 1.24.13 tests and race checks, the Next.js production build, and authenticated end-to-end staging acceptance must pass before launch. Some policy-dependent capabilities remain intentionally limited or deferred, including payment/refund rules, offline-authorised actions, billing, ownership transfer, impersonation, and destructive tenant deletion.

## Tenant security
Tenant identity is loaded from the authenticated server-side session and revalidated against active user, business, membership and branch records on each tenant-protected request. Browser-supplied business/branch IDs are used only during the explicit workspace-selection operation and are validated against membership. Tenant SQL must use the transaction-local context established by tenant middleware. See `docs/SECURITY_MODEL.md`.

## Authentication and request protections
- Login attempts are throttled by normalized email: 10 attempts per 15-minute window. Add edge/IP/device-aware controls before public launch.
- Passwords use bcrypt and are validated to remain within bcrypt's 72-byte limit.
- JSON requests enforce content type, body size, known fields and a single JSON value.
- Cookie-authenticated state changes require an exact `Origin` match with `WEB_ORIGIN`.
- Session cookies are HttpOnly and use configurable Secure and TTL settings. Set `SESSION_COOKIE_SECURE=true` in production and deploy behind HTTPS.

## Database roles and RLS
The Goose migration connection uses a tightly controlled database migration owner. The API connects as the separate `mekuin_app` role. Compose provisions its password from `MEKUIN_APP_PASSWORD`; no application-role password is embedded in migrations. Do not run the API as the database owner or a BYPASSRLS role.

RLS is enabled and forced for `businesses`, `app_users`, `user_sessions`, `branches`, `memberships`, `membership_branches` and `audit_logs`. Login and opaque session lookup use narrowly-scoped SECURITY DEFINER functions. Their owner must be the controlled migration principal with the required RLS bypass capability; do not grant that capability to `mekuin_app`. Transaction-local context is defense in depth against accidental unscoped SQL, not protection against arbitrary SQL execution by a compromised application role.

## Workspace selection
After login or registration, load `GET /api/v1/workspace`, then submit the chosen `business_id` and `branch_id` to `POST /api/v1/tenant/select`. The selection is stored on the current server-side session. Subsequent tenant-protected routes load that selection from the session; client-supplied tenant headers are not authorization. `GET /api/v1/tenant/context` is a diagnostic endpoint.

## Phase 1 security and deployment notes

See [`docs/PHASE_1_ACCEPTANCE_AND_OPERATIONS.md`](docs/PHASE_1_ACCEPTANCE_AND_OPERATIONS.md) for runtime role provisioning, authorization lifecycle, required acceptance tests, and remaining security gaps. Configure `MEKUIN_APP_PASSWORD` before starting Compose; the one-shot `runtime-role` service applies it after migrations and before the API starts.

## Phase 2A — catalog schema

The Phase 2A schema is delivered in `db/migrations/008_catalog_configuration.sql`. Apply it with the migration owner after migrations 001–007, then run both `db/tests/tenant_rls_smoke.sql` and `db/tests/catalog_rls_smoke.sql` as described in the operations guide. The API runtime role must remain `mekuin_app`; do not run application queries using the migration owner.

This increment is database foundation only. It does not yet expose catalog CRUD endpoints or management screens. Those are Phase 2B and 2C respectively.

## Phase 2A — catalog schema

The Phase 2A schema is delivered in `db/migrations/008_catalog_configuration.sql`. Apply it with the migration owner after migrations 001–007, then run both `db/tests/tenant_rls_smoke.sql` and `db/tests/catalog_rls_smoke.sql` as described in the operations guide. The API runtime role must remain `mekuin_app`; do not run application queries using the migration owner.

This increment is database foundation only. It does not yet expose catalog CRUD endpoints or management screens. Those are Phase 2B and 2C respectively.

### Phase 3H progress

- v48: Daraja transport foundation (OAuth, STK Push and STK Push Query).
- v49: durable tenant-scoped payment-attempt and callback-event schema.
- v50: strict STK callback parsing and idempotency fingerprint helper. This is not an enabled M-Pesa checkout flow; see `docs/PHASE_3H_DARAJA_CALLBACK_PARSING.md` for the safety boundary.


## Phase 3H — Daraja checkout and settlement (v53–v54 WIP)

- v52: authenticated provider reconciliation; `provider_confirmed` is not settled.
- v53: guarded atomic settlement verifies the tender, completes the sale, deducts recipe-backed stock and audits in one tenant transaction.
- v54: checkout accepts a single full-value M-Pesa tender, creates a pending sale and attempt, and initiates STK Push. Cash checkout remains unchanged.

See `docs/PHASE_3H_DARAJA_RECONCILIATION.md` and `docs/PHASE_3H_DARAJA_PENDING_CHECKOUT.md`. v54 is not production-ready: synchronous STK dispatch has a pre-commit callback race and ambiguous network outcomes require manual review. Split rails and table-order M-Pesa checkout are not enabled.

## M-Pesa payment lifecycle

- M-Pesa callback race recovery: unmatched callbacks are durably quarantined until both Safaricom request identifiers are committed, then correlated without resending STK Push.

### Phase 3H acceptance gate (v58)

- v58 expands conservative STK query-result handling for known in-progress
  subscriber/session responses and additional known terminal outcomes.
- Added `docs/PHASE_3H_SANDBOX_ACCEPTANCE.md`, a repeatable sandbox scenario
  matrix and explicit production go-live evidence gate.
- This is not a claim of a completed Safaricom sandbox run. Production M-Pesa
  remains blocked until Go 1.24.13, PostgreSQL 17, and real sandbox lifecycle
  checks pass with retained evidence.

### Phase 3H provider contract and sandbox preflight (v59)

The Daraja STK Push client now has deterministic HTTP contract tests covering OAuth, request fields, timestamp/password construction, provider correlation IDs and local validation. Before running the API against sandbox credentials, use `sh scripts/mpesa-sandbox-preflight.sh`; it validates configuration shape only and does not perform a provider transaction. See `docs/PHASE_3H_SANDBOX_ACCEPTANCE.md` for the actual sandbox evidence gate.

## Phase 4A — Inventory operations

The current WIP adds ledger-derived branch balances, wastage/breakage movements, and atomic paired branch transfers. See `docs/PHASE_4A_INVENTORY_OPERATIONS.md`. Purchase orders and receiving remain the next Phase 4 increment.

### Phase 5A — Cash shifts and drawer reconciliation

The current increment adds branch-scoped cash shift opening/closing, reasoned paid-in/out movements, cash tender attribution, variance calculation and a printable shift summary page. Cash checkout now requires an open shift. See `docs/PHASE_5A_CASH_SHIFTS.md` for the calculation, access model and explicit refund/expense boundaries.


### Phase 5B — Expense management and cash integration

Adds branch expense capture, category management, manager approval/rejection, payment recording and category/status reporting. Cash expense payment is atomically linked to the active cash shift as one paid-out movement; non-cash payment methods do not affect drawer cash. See `docs/PHASE_5B_EXPENSE_MANAGEMENT.md` for workflow, permissions, acceptance scenarios and remaining boundaries.


## Phase 5C — Operational alerts and audit ledger

Phase 5C adds branch-scoped operational alert inboxes for inventory thresholds, pending expense/supplier-invoice approvals and cash variances, plus acknowledgement/resolution history and an immutable business audit ledger. See `docs/PHASE_5C_ALERTS_AUDIT_LEDGER.md` and `docs/PHASE_5C_VERIFICATION.md` for API contracts, rule boundaries and verification status.

### Phase 5D — Branch operational settings

Phase 5D adds branch-level operational alert controls and a configurable cash variance severity threshold. Managers and owners can manage settings at `/settings`; changes are audited. See `docs/PHASE_5D_OPERATIONAL_SETTINGS.md` and `docs/PHASE_5D_VERIFICATION.md`.
