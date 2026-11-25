# Phase 4B — Procurement API implementation increment

## Implemented in this increment

- `GET /api/v1/purchasing/orders` — active-tenant and selected-branch scoped list with status/search/pagination.
- `POST /api/v1/purchasing/orders` — creates a draft PO and its product lines; validates active supplier/products and rejects duplicate products.
- `POST /api/v1/purchasing/orders/{orderID}/order` — draft-to-ordered transition.
- `POST /api/v1/purchasing/orders/{orderID}/receive` — accepts an `Idempotency-Key`, locks the PO, checks replay fingerprint, locks product/branch inventory advisory keys in sorted order, checks outstanding quantities, appends `purchase_receipt` movements, updates received quantities/status, and writes audit metadata.
- Migration 031 grants only the column-level updates required for PO lifecycle and received quantity counters; procurement records remain protected from general update/delete.

## API payloads

Create PO:
```json
{"supplier_id":"uuid","po_number":"PO-001","notes":"optional","lines":[{"product_id":"uuid","quantity":"12","unit_cost_minor":45000}]}
```

Receive:
```json
{"note":"Delivery note","lines":[{"purchase_order_line_id":"uuid","quantity":"5"}]}
```
Header: `Idempotency-Key` (8–128 printable ASCII characters).

## Important implementation boundary

The handlers rely on the existing tenant transaction middleware (`catalogTx`) so response writes are committed/rolled back by the surrounding request transaction. This must be confirmed in a PostgreSQL-backed integration run. No database migration was executed in this environment.

## Not yet delivered

- PO detail endpoint and receipt history response.
- Cancel endpoint and cancellation rules.
- Supplier balance/payables accounting.
- Frontend purchasing screens.
- Full integration tests and database execution.

## Verification

The repository requires Go 1.24.13. The available environment has Go 1.23.2; `go test ./...` cannot start because the toolchain requirement is not met. PostgreSQL/Docker integration tests were not run. Do not treat this increment as production-verified.

## Follow-on increment: detail, receipt history and cancellation

Added after the initial receiving API package:

- `GET /api/v1/purchasing/orders/{orderID}` returns the tenant/branch-scoped PO header, product lines (ordered, received and remaining quantities) and receipt headers.
- `POST /api/v1/purchasing/orders/{orderID}/cancel` permits cancellation only while status is `draft` or `ordered`, and only when no receipt records exist. It locks the PO row and writes an audit event.
- Both endpoints require `purchasing.manage`.

### Verification boundary

The new handlers were formatted with `gofmt`. Automated Go tests could not run because the environment has Go 1.23.2 while the module declares Go 1.24.0 and toolchain 1.24.13. No PostgreSQL instance was available and no migrations were executed. The new SQL paths therefore still require PostgreSQL-backed integration tests, particularly RLS, runtime column grants, branch scoping, cancellation-vs-receiving races, and response serialization.
