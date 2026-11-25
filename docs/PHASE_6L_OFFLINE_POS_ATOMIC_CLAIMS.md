# Phase 6L — Atomic offline POS draft claims

## Changes

- Added `claimDraft`, an IndexedDB read/write transaction that reads the current outbox row and atomically changes an eligible `pending` or stale `syncing` draft to `syncing` before any create-order network request.
- A recent `syncing` record is not claimable. This protects concurrent tabs even where the Web Locks API is unavailable; Web Locks remains an additional same-origin coordination layer.
- Claiming increments the attempt count and appends a bounded diagnostic event in the same transaction.
- The sync loop now sends only the freshly claimed record, rather than replaying a stale snapshot obtained earlier in the reconciliation pass.
- Stable `client_reference` values and server-side idempotency remain the final protection against ambiguous network outcomes.

## Safety boundaries

- This does not make browser storage a distributed lock or guarantee coordination across devices/browsers.
- Stale recovery still uses a two-minute client-side heuristic. A request that runs longer than that can be replayed; server idempotency is still required.
- Payments, completed sales, stock movements, cash shifts and approvals remain online-only.

## Acceptance scenarios

1. Two tabs attempt to sync the same pending draft simultaneously; only one obtains the atomic claim.
2. A second tab sees a recent `syncing` row and skips it.
3. A `syncing` row older than the stale threshold can be claimed and retried with its original reference.
4. A browser closes after the server commits but before the response; reconciliation or idempotent replay confirms the original order.
5. A failed IndexedDB claim transaction prevents the create-order request from being sent.
6. A blocked or synced draft cannot be claimed.
7. Confirm that each claim appends a bounded history event and increments attempts exactly once.

## Verification status

Source-level TypeScript/TSX transpilation and archive integrity are checked for this delivery. Full TypeScript type-check, browser concurrency tests, Go tests, PostgreSQL/RLS checks and production Next.js build remain external acceptance tasks.
