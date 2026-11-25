# Phase 4B — Procurement Read-Path Hardening

## Scope

This increment adds narrowly scoped PostgreSQL indexes for the purchase-order detail and receipt-history read paths.

## Changes

- `purchase_receipts_branch_history_idx` supports branch-scoped receipt history ordered newest first.
- `purchase_receipt_lines_receipt_history_idx` supports expansion of receipt line details by receipt ID in stable creation order.
- Migration `032_procurement_read_path_indexes.sql` includes a reversible down section.

## Invariants preserved

- No table or API contract changes.
- No changes to receipt posting, idempotency, quantity arithmetic, or inventory movement semantics.
- No additional write privileges.
- Existing tenant RLS policies remain authoritative.

## Verification

- Reviewed index column order against the queries in `purchasing.go`.
- ZIP archive integrity checked after packaging.
- Migration has not been applied to a live PostgreSQL database in this environment.
- Full Go tests remain blocked because the environment has Go 1.23.2 and cannot download the required Go 1.24.13 toolchain.
- Query plans should be checked with representative production-sized data using `EXPLAIN (ANALYZE, BUFFERS)` after applying the migration in a test database.
