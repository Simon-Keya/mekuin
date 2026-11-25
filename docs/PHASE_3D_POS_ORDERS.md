# Phase 3D — POS order lifecycle (v44)

## Implemented
- Branch-scoped `pos_orders` and `pos_order_lines` with tenant RLS and FORCE RLS.
- Service types: `walk_in`, `table`, `tab`; table orders require an active table assigned to the selected branch.
- Create an order using server-loaded product prices and immutable-at-this-stage line snapshots.
- List the branch's open and held orders; retrieve a single order and its lines.
- Hold an open order. State changes are audited.
- Orders do not create sales, payments, revenue, or stock movements. Stock is consumed only by transactional checkout.

## API
- `POST /api/v1/sales/orders`
- `GET /api/v1/sales/orders`
- `GET /api/v1/sales/orders/{orderID}`
- `POST /api/v1/sales/orders/{orderID}/hold`

All routes require `pos.use` and derive business/branch scope from the authenticated tenant context.

## Not yet implemented
- Editing/appending/removing order lines.
- Reopening held orders, cancelling orders, table occupancy/transfer, tab customer/credit settlement.
- Checkout directly from an order (must atomically bind the order to the resulting sale and consume stock exactly once).
- Offline order synchronization.

## Verification status
Source formatting and static file checks are performed for this archive. Go tests, PostgreSQL 17 migrations/RLS tests, API HTTP smoke tests and frontend build require CI or the user's local environment; they are not claimed as executed here.


## Phase 3E lifecycle extension (v45)

- `PUT /api/v1/sales/orders/{orderID}` replaces lines on an open order. Product prices and snapshots are reloaded server-side; held/terminal orders cannot be edited.
- `POST /api/v1/sales/orders/{orderID}/reopen` resumes a held order.
- `POST /api/v1/sales/orders/{orderID}/cancel` cancels an open or held order without creating a sale or stock movement.
- Migration 019 enforces one active table order per physical table and branch.
- Every transition and line replacement is audited. Row locks serialize order edits and state transitions.

Direct order-to-sale checkout, split tenders, table transfer and table occupancy release after checkout remain future work. The regular checkout endpoint remains the only completed-sale path in this increment.
