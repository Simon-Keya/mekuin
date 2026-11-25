# Phase 6C — Offline POS scope safety, tables and recovery safeguards

## Implemented

- Fetch the authenticated tenant context immediately before replaying the outbox. A queued draft is replayed only when the server's current business and branch exactly match the draft's stored scope.
- Scope-mismatched pending/recovered `syncing` drafts are blocked locally with an actionable explanation; no request is sent to the wrong branch.
- Preserve HTTP status in API errors. Definitive HTTP 4xx responses block a draft; transport errors and 5xx responses remain pending for idempotent retry.
- Coordinate queue replay across tabs with the browser Web Locks API when supported, retaining the per-page mutex as an additional guard.
- Add branch-scoped IndexedDB table caching (database schema version 2, preserving existing stores and queued records).
- Add read-only POS table listing under `pos.use`; table administration endpoints remain protected by `tables.manage`.
- Enable table order drafts using a selected table from the current online table list or the last branch-scoped cached list. The UI warns when the list is stale; server-side active-table validation remains authoritative on sync.
- Surface browser storage quota errors without automatically deleting pending, syncing or blocked drafts. Add a manual bulk-clear action for synced local entries only.
- Await blocked-draft deletion before restoring its content to the basket, avoiding a save/delete race.

## Safety boundaries

- The selected business/branch is always taken from authenticated server context when online. Cached context is only a local display/offline capture aid and is never accepted as server authority.
- Table IDs are not trusted because they came from cache. The existing order endpoint revalidates that the table is active in the authenticated branch.
- Cached prices remain indicative only; the server continues to revalidate product availability and pricing.
- Offline payments, completed sales, stock deduction, cash-shift movement, refunds, discounts and approvals remain out of scope.
- If Web Locks is unavailable, the in-page mutex remains active, but cross-tab mutual exclusion is not guaranteed by this fallback.

## Still outstanding

- Browser acceptance for offline/online transitions, scope switching, stale/deactivated tables, concurrent tabs, quota exhaustion and timeout-after-commit recovery.
- Device-wide retention policy and explicit operator-controlled clearing of old blocked records.
- External Go 1.24.13, PostgreSQL 17, Next.js production build and authenticated staging checks.

## Acceptance boundary

This is a source implementation increment, not production acceptance. External integration and browser checks remain with the operator.
