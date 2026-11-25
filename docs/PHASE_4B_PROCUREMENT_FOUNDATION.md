# Phase 4B — Procurement foundation (schema increment)

## Delivered in this slice
- Tenant-scoped purchase order header records with branch, supplier, PO number, status and author metadata.
- Purchase order lines with ordered quantity, unit cost in integer minor units, and received quantity tracking fields.
- Immutable receipt headers and receipt line records, each receipt line linked to its PO line and intended inventory movement identifier.
- `purchase_receipt` movement type added to the immutable inventory ledger constraint.
- Forced tenant RLS and runtime SELECT/INSERT grants; UPDATE/DELETE remain revoked on the procurement tables.
- Status vocabulary: draft, ordered, partially_received, received, cancelled.

## Deliberately not yet delivered
- HTTP handlers or frontend screens.
- Atomic receiving service that writes movements and increments received quantity.
- Supplier balance/payments or cost valuation policy.
- Editing/cancellation API or PO number generation strategy.

## Required next slice
Implement API handlers for PO creation/list/detail, order transition, and receiving. Receiving must validate tenant/branch/supplier/product/line membership, lock PO and product rows/advisory locks, reject over-receipts and inactive products, write a receipt header, receipt lines and purchase_receipt movements, update received quantities and status, and write audit log in one transaction. Add an idempotency key to the receive operation and tests for replay, partial receipt, over-receipt, rollback and concurrent receipt attempts.

## Acceptance status
Schema only. Migration has not been executed against PostgreSQL in this environment. Go tests and database smoke tests are not claimed to pass.

## Follow-up design slice (2026-11-24)
- Added migration 030 for idempotency-key and request-fingerprint columns and uniqueness scoped to business + PO.
- Added the receiving API contract and transactional acceptance scenarios in `docs/PHASE_4B_RECEIVING_API_CONTRACT.md`.
- Extended role permission maps to grant procurement manage/receive to owner, manager and stock-controller roles.
- HTTP endpoints and transactional receiving handler are still pending; migration 030 is not executed here.
