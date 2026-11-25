# Phase 6D — Offline POS recovery and acceptance hardening

## Implemented in this increment

- Recover locally persisted `syncing` drafts to `pending` during POS initialization. This covers a tab close, browser crash or device restart between marking a draft as syncing and recording its result.
- Preserve the original `client_reference` during recovery. The server's idempotency contract remains the protection against a timeout-after-commit replay creating a duplicate order.
- Prevent saving a basket containing product IDs absent from the currently loaded active/sellable product cache. This is a local guard only; the server remains authoritative and can still reject a product that was deactivated after the last cache refresh.
- Label the basket total as indicative/cached and make the price/product/table revalidation boundary explicit in the POS UI.

## Manual browser acceptance matrix

Run these scenarios in a non-production test business and branch. Record browser, OS, result and any server order IDs.

| Scenario | Procedure | Expected result |
|---|---|---|
| First online load | Open POS with an authenticated `pos.use` user | Context, sellable products and active tables load; scoped cache is written |
| Offline capture | Load POS online, disconnect, create a walk-in draft | Draft is retained locally; no sale/payment/stock movement is created |
| Reload offline | Reload while disconnected | Cached context/products/tables and basket/outbox remain available |
| Interrupted sync | In test tools, leave a local draft in `syncing`, then reload POS | Draft is recovered to `pending` with the same client reference |
| Timeout after commit | Interrupt the response after server commit, then retry | Exactly one server order exists for the client reference; local draft becomes synced |
| Scope switch | Queue in branch A, authenticate into branch B, then attempt sync | A's draft is not posted to B; it remains blocked until the original scope is restored |
| Deactivated product | Queue while product is active, deactivate it server-side, then sync | Server rejects the draft; no order is created; operator can correct/discard locally |
| Changed price | Queue at one price, change server price, then sync | Server's pricing rules decide acceptance/price; cached total is never treated as authoritative |
| Stale table | Queue a table draft, deactivate the table, then sync | Server rejects the stale table; no order is created |
| Concurrent tabs | Open two POS tabs and trigger sync in both | Web Locks serialize replay where supported; idempotency still prevents duplicate server orders |
| No Web Locks | Test a browser without Web Locks support | Per-tab lock works; document that cross-tab exclusion is not guaranteed in this fallback |
| Storage quota | Simulate IndexedDB quota failure while queueing | Clear error is shown; no pending draft is silently deleted |
| Clear synced | Clear synced local records | Only synced local outbox entries are removed; server orders remain unchanged |
| Permissions | Open POS without `pos.use` | POS is denied; no draft replay is initiated from the denied UI |

## Operational cautions

- Do not manually alter client references when investigating a timeout. The stable reference is essential for idempotent replay.
- Never auto-delete pending, syncing or blocked drafts. Operators must explicitly discard blocked work after reviewing it.
- A locally cached product/table list is not an authority. A successful local save means only that a draft was recorded on this device.
- The current product guard only detects missing products in the loaded cache. A product can be deactivated remotely after the cache was refreshed; server validation remains mandatory.
- Offline payments, completed offline sales, stock deduction, refunds, cash-shift attribution and approvals remain disabled/out of scope.

## External verification still required

- Go 1.24.13 tests, race tests, vet and build.
- PostgreSQL 17 migration application, RLS and idempotency/concurrency integration tests.
- Next.js production lint/type/build.
- Authenticated browser/staging acceptance using the matrix above.

This phase is implementation hardening, not a production-readiness declaration.
