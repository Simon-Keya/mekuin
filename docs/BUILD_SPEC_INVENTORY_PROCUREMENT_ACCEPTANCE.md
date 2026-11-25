# Build-spec alignment: inventory and procurement API acceptance

This acceptance slice is derived from **MEKU-In Developer Build Specification and Screen Booklet v1.0**, not from the ZIP's historical phase labels.

## Specification references

- **Page 3 – Feature map:** Inventory must expose live stock by branch, movement history, wastage, breakage, adjustments and transfers. Stock taking and reconciliation are distinct stock-control capabilities. Purchases follow `draft > ordered > received`, and receiving posts stock movements; suppliers have a directory and balances.
- **Page 4 – Data model:** `StockMovement` is the record of every stock change; `StockCount` stores expected, counted and variance quantities; `Supplier` and `PurchaseOrder` represent supply-side records.
- **Page 6 – Suggested build order and acceptance:** Phase 4 is inventory, wastage/breakage/transfer, purchases/receiving, stock taking and reconciliation. The completion criterion is that count variance and leakage reports reproduce a known test scenario.
- **Page 16 – Inventory screen:** On-hand is the sum of stock movements per product and branch; transfers create paired out/in movements; inventory has on-hand, movement, wastage, breakage and transfer views.
- **Page 17 – Stock taking screen:** Blind counts hide expected quantities; rows calculate variance and cost/retail impact; manager approval posts variance movements and an audit entry.
- **Page 18 – Reconciliation screen:** Theoretical usage comes from sales and recipes; actual usage comes from opening stock, purchases and closing counts; leakage value and signals are shown by product.
- **Page 19 – Purchases screen:** Purchase orders progress through draft, ordered and received; partial receipts are allowed; receiving posts stock movements and updates supplier balance.

## Work added in this slice

`db/tests/api_inventory_procurement_smoke.sh` adds authenticated HTTP integration coverage for the implemented inventory and purchase-order endpoints. The test is wired into the PostgreSQL 17 CI job immediately after the existing catalog API smoke test.

Coverage added:

1. Creates a tenant, branch, products and supplier through authenticated APIs.
2. Seeds opening quantities into the immutable movement ledger and checks branch-scoped on-hand balances.
3. Verifies a multi-line wastage request is atomic: an oversubscribed second line must not leave the first line committed.
4. Exercises successful wastage and breakage and rejects oversubscription.
5. Rejects transfers to a branch not assigned to the acting user.
6. Verifies a permitted transfer creates exactly two ledger entries (`transfer_out` and `transfer_in`) and that the destination branch balance reflects the inbound quantity.
7. Exercises purchase-order creation, ordering, partial receipt, exact idempotent replay, mismatched replay rejection, over-receipt rollback, completion and cancellation restrictions.
8. Checks that completed receipts increase the source branch's ledger balance by exactly the received quantity.

## What this does not claim

- This is integration-test coverage for existing endpoints; it does not implement missing screen functionality or assert that all inventory UI requirements are complete.
- It does not establish weighted-average costing, formal stock valuation, or supplier balance accounting. The purchase-receipt handler appends receipt movements and updates PO receipt state; the supplier entity itself has no balance column, and this handler does not create a supplier payable. The specification says receiving updates supplier balance (page 19), so the intended relationship between receipts, supplier invoices and the supplier balance still needs an explicit implementation decision and acceptance test.
- The current inventory route set exposes balances, wastage/breakage and transfers, but this slice does not add the movement-history view or the general manual adjustment action shown on page 16.
- The current stock-count API snapshots expected and counted quantities and manager approval posts reconciliation movements. It does not yet demonstrate the page 17 blind/open modes, count area/header workflow, or cost/retail impact presentation.
- The page 18 theoretical-versus-actual reconciliation and leakage-value report is not covered by this integration slice. These remain separate build-spec acceptance items unless verified elsewhere in the repository.
- The smoke test requires the CI PostgreSQL 17 service, a running API using `mekuin_app`, and the migration-owner connection for deterministic opening-stock fixtures. It cannot be fully executed in the current local environment where PostgreSQL client/server tooling is unavailable.

## Execution

CI runs:

```sh
sh db/tests/api_inventory_procurement_smoke.sh
```

Required environment:

- `BASE_URL` (defaults to `http://127.0.0.1:8080`)
- `WEB_ORIGIN` (defaults to `http://localhost:3000`)
- `MIGRATION_DATABASE_URL` (schema-owner connection for test fixtures)

The API must be running with its runtime database connection configured as `mekuin_app`.
