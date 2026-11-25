# Phase 6F — Offline POS basket and catalogue safeguards

## Scope

Phase 6F hardens the operator-facing order-draft workflow. It does not introduce offline payment capture, completed sales, stock movements, cash-shift movements, or approval workflows.

## Implemented

- Added an online **Refresh catalogue** action that refreshes products and tables together and writes each result to the active business/branch cache.
- Catalogue refresh validates the response shape and excludes product rows with invalid or unsafe integer minor-unit prices.
- The initial online catalogue load applies the same safe-price filter before caching products.
- Basket quantity controls enforce a per-line maximum of 10,000 units; quantity and money calculations are checked with JavaScript safe-integer rules before a draft can be saved.
- Saving a table-service draft requires the selected table to exist in the active cached table list. The API remains authoritative and revalidates the table on sync.
- Sync only marks a draft as synced when the API returns a non-empty `order_id`. An incomplete success response leaves the same idempotency reference queued for safe retry.
- The UI makes catalogue refresh availability and stale cached table context visible to the operator.

## Safety invariants retained

- Drafts remain scoped to their original business and branch.
- Server-side authentication, permissions, price validation, product availability and table validation remain authoritative.
- A draft is not a sale and does not capture payment, recognise revenue, deduct stock or move cash.
- Pending drafts are not automatically deleted to reclaim browser storage.
- Retries retain the original `client_reference` so an ambiguous timeout can be reconciled idempotently.

## Verification

- TypeScript/TSX syntax transpilation is performed for the modified POS page.
- ZIP integrity is checked for the delivery archive.
- Full Next.js build, Go tests, PostgreSQL migration/RLS tests, and browser/staging acceptance are external checks and are not claimed by this implementation report.

## Follow-up

- Browser acceptance should exercise catalogue refresh while online, refresh failure, stale cached tables, maximum quantity, unsafe total handling, successful order creation, and a 2xx response missing `order_id`.
- Confirm the API's exact successful create-order response contract in staging.
