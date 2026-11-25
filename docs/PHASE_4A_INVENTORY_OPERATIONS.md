# Phase 4A — Inventory operations

This increment starts Phase 4 (Stock) from the MEKU-In build specification.

## Delivered

- `GET /api/v1/inventory/balances`: branch-scoped on-hand quantities derived from the immutable inventory movement ledger; no independently writable balance table is introduced.
- `POST /api/v1/inventory/losses/wastage` and `/breakage`: records negative movements with a required reason; rejects zero/negative quantities, duplicate product lines, inactive/missing products and stock oversubscription.
- `POST /api/v1/inventory/transfers`: records paired `transfer_out` and `transfer_in` movements in one tenant transaction, checks the destination branch is active and assigned to the acting user, and prevents negative source stock.
- Product rows are locked in deterministic product-ID order before checking and writing stock, serializing competing operations for the same product.
- Migration 028 extends the movement type constraint and adds transfer duplicate protection.
- Runtime role retains SELECT/INSERT only on the immutable ledger; UPDATE/DELETE remain revoked.

## Permissions

- Inventory balances: `inventory.read`.
- Loss recording and transfers: `inventory.adjust` (owner/manager in current role map).
- Physical count creation/reconciliation remains on the existing count endpoints and manager-approval flow.

## Deliberate boundaries

- Transfers are immediate paired ledger movements; this increment does not model an in-transit state or receipt confirmation.
- No valuation/costing policy is introduced. Quantities remain exact NUMERIC values.
- Inventory balances are derived from all ledger movements for the selected branch and active products.
- Purchase orders, partial receiving, supplier payments and stock valuation remain subsequent Phase 4 work.
- No production-readiness claim is made until PostgreSQL 17 and full Go CI execute.

## Acceptance scenarios for CI / staging

1. A branch with opening stock 20 records wastage 2; balance becomes 18.
2. Breakage exceeding on-hand stock returns 409 and leaves no movement.
3. A transfer of 5 from branch A to assigned branch B creates one negative and one positive movement with the same reference ID.
4. A destination branch not assigned to the actor is rejected.
5. Concurrent transfers/losses against the same product cannot oversubscribe stock because the product row is locked.
6. Runtime role cannot update or delete inventory movements.

## Concurrency and count reconciliation

All stock-changing endpoints in this increment use the same transaction-scoped advisory-lock key convention as recipe sale consumption (`business:branch:stock:product`). Transfers acquire source and destination branch locks in deterministic branch order. Physical-count reconciliation now recalculates current ledger on-hand after acquiring the same product lock, rather than blindly applying a delta against the stale count-start snapshot. This prevents sales or other movements posted during a count from being overwritten by the count reconciliation.
