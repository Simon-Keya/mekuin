# Go API

The API currently establishes the service runtime, Chi middleware, health/readiness endpoints, database pool and OpenAPI entry point. Business endpoints are added by domain modules in subsequent implementation steps.

Conventions:
- domain packages own business rules
- HTTP handlers validate input and map errors
- service layer owns transactions
- repositories use pgx/sqlc
- tenant-scoped queries run in a transaction with `SET LOCAL app.business_id = $1`
- never trust a tenant ID supplied by the browser

Tenant-scoped use cases must call `withTenantTx` with the authenticated user ID
and requested business/branch identifiers. The helper validates membership in
the database before setting transaction-local RLS context. Call `hasPermission`
inside the use case before performing role-restricted actions. Never use the
helper with IDs copied from an untrusted request without the membership check.


### Daraja reconciliation

`POST /api/v1/payments/mpesa/{attemptID}/reconcile` requires `pos.use`, tenant and branch context, and the configured Daraja environment. It records provider query results and only advances a matching successful attempt to `provider_confirmed`. It does not settle a sale or deduct stock.


### Pending M-Pesa checkout (v54 WIP)

`POST /api/v1/sales/checkout` accepts either the existing cash tender or one full-value M-Pesa tender. For M-Pesa, send `payments: [{"method":"mpesa","amount_minor":12300,"phone_e164":"2547XXXXXXXX"}]`. The amount must be whole KES and equal the server-calculated sale total. The API creates a `pending_payment` sale and defers recipe stock consumption until the separate reconciliation and atomic settlement steps complete. Split rails and order checkout are rejected. See `docs/PHASE_3H_DARAJA_PENDING_CHECKOUT.md`; do not enable production payments until the callback race and ambiguous-initiation recovery are addressed.

## Phase 4A — Inventory operations

The current WIP adds ledger-derived branch balances, wastage/breakage movements, and atomic paired branch transfers. See `docs/PHASE_4A_INVENTORY_OPERATIONS.md`. Purchase orders and receiving remain the next Phase 4 increment.

## Phase 4B — Procurement

Purchase-order lifecycle and partial receiving are implemented under `/api/v1/purchasing/orders`. Receiving uses an idempotency key and appends immutable purchase-receipt inventory movements. See the Phase 4B procurement documentation in `docs/`.

## Phase 4C — Supplier payables API

Supplier invoices and payment allocations are available under `/api/v1/payables`. Invoice writes validate line and header totals; payment writes lock affected invoice rows in deterministic order and reject allocations above outstanding balances. Invoice and payment records remain separate from inventory receipt movements and no general-ledger journals are posted. See `docs/PHASE_4C_SUPPLIER_PAYABLES_API.md` and `docs/PHASE_4C_SUPPLIER_PAYABLES_FOUNDATION.md`.

## Supplier payables reporting (Phase 4D)

The finance-read API exposes branch-scoped supplier statements at
`GET /api/v1/payables/suppliers/{supplierID}/statement`, ageing at
`GET /api/v1/payables/reports/aging`, and internal payment-allocation
reconciliation at `GET /api/v1/payables/reports/reconciliation`. See
`docs/PHASE_4D_SUPPLIER_STATEMENTS_RECONCILIATION.md` for date, currency and
accounting semantics. The reconciliation report is not a bank/provider feed
reconciliation.
