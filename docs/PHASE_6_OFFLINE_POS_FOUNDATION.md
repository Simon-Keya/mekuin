# Phase 6 — Offline POS foundation

## Implemented in this increment

- Added an IndexedDB-backed product cache and durable POS order-draft outbox. Local browser storage is not encrypted by this feature; avoid putting sensitive personal information in the optional label.
- Outbox records are scoped by the authenticated business and selected branch; the UI only displays and replays the active scope.
- Added `/pos` with cached sellable-product browsing, a basket that is persisted per business/branch, local order-draft capture, online/offline state, pending/synced/blocked states, manual retry and automatic replay when connectivity returns.
- Added a service worker that caches the POS/workspace navigation shell after a successful visit and caches Next.js static assets; API/auth requests are never cached. Cached active branch context is used only to permit local draft capture while offline; server authorization is still required for replay.
- Added a stable UUID `client_reference` to queued order drafts. The API hashes the canonical order payload, serializes replay by tenant/branch/reference, returns the original order on an identical replay, and rejects reuse of a reference with a different payload.
- Added migration `040_offline_pos_client_references.sql` with a tenant/branch-scoped unique index and payload-hash consistency constraint.
- Added the POS entry point to the workspace for users with `pos.use`.

## Offline action boundary

This increment intentionally queues **order drafts only**. Offline capture does not create a sale, record or verify a payment, recognise revenue, consume stock, close a shift, apply discounts, void/refund, or approve any manager-controlled action. Prices shown from the device cache are indicative; the server recalculates prices and rechecks product/table availability on replay. Table orders are disabled in the offline form because table availability cannot be validated without the server. Checkout and all payment operations remain online-only.

This is a conservative first boundary while the business's offline payment, cash-shift and approval rules remain undecided. It avoids treating a local browser record as a completed financial transaction.

## Recovery behaviour

- A queued draft is written to IndexedDB before the UI reports it saved.
- A replay includes the same `client_reference` on every attempt.
- If the server committed but the browser lost the response, replay returns the already-created order rather than creating another.
- A network/server failure remains pending; validation, permission and conflict errors become blocked and require user review/retry.
- Synced records remain locally visible as a device history; they are not deleted automatically.

## Remaining Phase 6 work

- Add offline table cache and a safe table-selection workflow after freshness/occupancy rules are agreed.
- Add explicit storage quota/eviction handling and a deliberate stale-cache refresh indicator.
- Add browser-level IndexedDB, airplane-mode, lost-response, concurrent-replay and tenant-switch acceptance tests.
- Decide whether offline cash tender capture is permitted, how cash is attributed to shifts, what approval boundaries apply, and how stale prices/stock are presented.
- Decide conflict resolution for an order whose product is deactivated or repriced before replay.

Phase 6 is an incremental foundation, not a declaration that all offline checkout is enabled.
