# Phase 4B — Receipt traceability and replay hardening

## Included in this increment

- Normalize receipt line UUIDs and quantity strings in the request slice before sorting and hashing. This makes idempotency fingerprints stable when a client retries with equivalent UUID casing or surrounding whitespace.
- Keep receipt lines in a deterministic order before generating the request fingerprint.
- Expand purchase-order detail responses so each receipt includes its received product lines, quantities, unit costs, and linked inventory movement IDs.
- Update the procurement screen to show the received product quantities under each receipt in the receipt history.
- Return the idempotent replay indicator as a JSON boolean rather than a string.

## Transaction and security behavior

The existing `requireTenant` middleware executes tenant handlers inside a transaction and buffers responses. Responses with status 400 or greater cause rollback; successful responses are committed before being sent. Receipt creation therefore retains the existing atomic boundary across receipt headers, receipt lines, inventory movement inserts, purchase-order received quantities, and audit records.

The receipt detail query remains scoped by the active business ID and only loads receipts for the selected branch and purchase order. Receipt lines are additionally constrained by business ID.

## Verification

- `gofmt` applied to the modified Go source.
- `git diff --check` is run during packaging.
- Go test execution was attempted using the module's pinned Go 1.24.13 toolchain. The environment attempted to download that toolchain, but network DNS access to `proxy.golang.org` was unavailable. Tests therefore did not run.
- PostgreSQL migrations and integration scenarios have not been executed in this environment.
- The Next.js build has not been run because frontend dependencies are not installed and package installation is not available here.

This increment improves traceability and retry canonicalization; it does not establish production readiness. Complete the Phase 4B acceptance checklist against disposable PostgreSQL before deployment.
