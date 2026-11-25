# Build specification follow-through: inventory ledger and operations

This change is governed by the MEKU-In Developer Build Specification and Screen Booklet, especially the feature map (page 3), stock control data model (page 4), stock workflow build/acceptance sequence (page 6), and screen requirements for Inventory, Stock Taking, Reconciliation and Purchases (pages 16–19).

## Implemented in this increment

- Added `GET /api/v1/inventory/movements`, a paginated, branch- and tenant-scoped read of the immutable movement ledger.
- Added `POST /api/v1/inventory/adjustments`, a reasoned manual quantity adjustment that serializes against other stock operations, rejects zero/invalid deltas and prevents a negative on-hand balance.
- Added `manual_adjustment` as an explicit movement type in migration 043. Runtime permissions remain append-only: the API role receives no ledger UPDATE/DELETE capability.
- Added the Inventory screen at `/inventory` with branch on-hand quantities, movement history, manual adjustment, wastage, breakage and branch transfer actions.
- Added open/blind count modes and an optional count area in migration 044, with narrow runtime insert grants.
- Added migration 045 and transactional inventory reversal for sale voids and proportional partial refunds. Refund reversals are cumulative per ingredient and full voids restore only the remaining unreturned quantity.
- Added branch-scoped count list/detail APIs. Blind count details suppress expected quantities, variance and valuation impact until reconciliation.
- Added the Stock Taking screen at `/stock-taking`, with count capture, recent count history, variance review and separate manager PIN approval before atomic reconciliation.
- Added the stock reconciliation API and `/reconciliation` screen. The report compares recipe-consumption ledger movements with actual usage derived from a reconciled opening count, posted receipts and a reconciled closing count; it withholds actual-use/leakage figures when either count baseline is missing.
- Added workspace navigation to Inventory and Stock Taking, and updated the OpenAPI contract and inventory schema smoke assertion.
- Extended authenticated API smoke coverage for manual adjustment, movement history, negative-stock rejection, blind-count privacy, missing reconciliation baselines, proportional refund reversals and full void reversals; CI now expects 49 migrations and runs the suite.

## Deliberate boundaries

- This increment does not implement inventory valuation or weighted-average costing. The build specification screen describes cost value, but no complete valuation ledger is introduced here.
- Draft editing and a formal submit-for-approval state remain incomplete; count creation is draft and manager approval reconciles it directly.
- The reconciliation screen is implemented, but its theoretical-use input currently uses the recipe-derived `sale_consumption` ledger produced during checkout. It does not independently recompute historical SaleItem × Recipe quantities from versioned recipe snapshots; that exact historical recomputation remains a verification/design gap. Supplier-payable reconciliation is a separate report and must not be represented as stock leakage reconciliation.
- Local PostgreSQL execution is required to validate migrations 043–045 and the HTTP integration smoke test. The current editing environment has no PostgreSQL or Docker.
