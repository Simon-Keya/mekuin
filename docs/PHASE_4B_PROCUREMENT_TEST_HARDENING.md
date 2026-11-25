# Phase 4B — Procurement test hardening

## Added in this increment

- Extracted receipt idempotency-key validation into a pure helper.
- Extracted receipt payload canonicalization and fingerprinting into a pure helper.
- Canonicalized decimal quantities so equivalent values such as `01.5000` and `1.5` fingerprint identically.
- Centralized purchase-order receivable and cancellable lifecycle predicates.
- Added Go unit tests for canonical fingerprint stability, invalid receipt lines, idempotency-key rules, and lifecycle eligibility.
- Added `db/tests/procurement_schema_smoke.sql` to verify tenant RLS, forced RLS, receipt-key uniqueness, narrow purchase-order update grants, and append-only inventory movement privileges after migrations 029–031.

## Verification performed

- `gofmt` completed for the procurement handler and its test file.
- The installed Go runtime is 1.23.2, while this project declares Go 1.24.0 and toolchain 1.24.13. The normal test command therefore stops before compilation.
- A local compatibility attempt with the `go` directive temporarily lowered also stopped because the project archive does not contain `go.sum` and the required modules are not cached. The original `go.mod` was restored unchanged.
- PostgreSQL is not available in this environment; the schema smoke test was added but not executed.
- No concurrent-request, HTTP integration, frontend, or migration execution tests are claimed as passing.

## Required CI / staging verification

1. Use the pinned Go 1.24.13 toolchain and resolve dependencies to produce a committed `go.sum`.
2. Run `go test ./...` from `services/api`.
3. Apply migrations 029, 030, and 031 to a disposable PostgreSQL database using the project's normal migration runner.
4. Run `psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f db/tests/procurement_schema_smoke.sql` with a role able to inspect privileges.
5. Add database-backed API tests for draft creation, ordering, partial receipt, complete receipt, replay with same payload, replay mismatch, over-receipt rollback, cancellation after receipt, and branch/business isolation.
6. Run two simultaneous receipt requests for the same PO and verify that inventory movements and receipt lines are posted exactly once per accepted idempotency key.

## Scope boundaries

This increment does not claim complete procurement integration coverage. The SQL smoke test verifies schema/security metadata only; it does not replace transaction-level API tests or real concurrent PostgreSQL tests.
