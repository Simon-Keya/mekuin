# Phase 6B — Offline outbox recovery and correction

## Scope

This increment extends the Phase 6A order-draft outbox. It does not turn an offline draft into a completed sale and does not enable offline payment, stock deduction, shift cash movement, table assignment, refunds, discounts, or manager approvals.

## Implemented

- Correct a server-rejected (`blocked`) draft by loading its lines, service type and label back into the POS basket.
- Remove the rejected draft before correction is saved. The corrected draft receives a fresh UUID client reference, avoiding reuse of a reference whose original payload may already have been observed by the server.
- Discard blocked drafts that cannot or should not be corrected.
- Clear locally retained records after successful server synchronisation without deleting the server-side order.
- Validate local basket quantities as whole numbers from 1 through 10,000 and reject duplicate product IDs before queueing.
- Add the missing synchronisation mutex ref so overlapping online/connectivity/manual sync triggers cannot run two queue replayers concurrently in the same page instance.

## Recovery and idempotency rules

- A `pending` or `syncing` draft is not offered for editing or deletion. A network failure may have occurred after the server committed the order; replay must retain the same client reference and exact payload.
- A `blocked` draft is only corrected after a definitive client/validation/permission/conflict response. Correction creates a new client reference.
- A `synced` entry can be cleared from this device; this only removes its local outbox record.
- A transport/server failure remains pending and is replayed with the original client reference.

## Still outstanding

- Table selection/cache and table availability policy.
- Cross-tab coordination for multiple browser tabs/devices sharing the same logical operator session.
- Queue retention/size limits, storage quota handling and explicit device-data clearing.
- Browser automation for airplane-mode, crash/reload, timeout-after-commit and duplicate replay scenarios.
- Product deactivation/price-change conflict UX beyond the server error and correct/requeue flow.
- Offline payments and cash-shift attribution require a separate explicit policy and are not included.

## Acceptance boundary

Phase 6B is source-implemented, not browser-accepted. External Go, PostgreSQL, Next.js build and authenticated staging checks remain the operator's responsibility.
