# Phase 6E — Offline POS operator recovery and draft inspection

## Scope

Phase 6E improves the operator's ability to understand and recover local order drafts. It does not introduce offline payments, completed offline sales, stock movements, cash-shift movements or approval workflows.

## Implemented

- Added an expandable inspection view for each outbox draft.
- Exposed the draft's business and branch identifiers, table assignment, server order reference, attempt count, timestamps and line items.
- Displayed indicative line values only when a matching cached product is available; unknown products and unavailable prices are called out instead of being guessed.
- Kept server order state and server-side price/product/table validation authoritative.
- Made destructive discard/clear actions use awaited error handling so IndexedDB failures are surfaced to the operator.
- Kept blocked-draft correction on the existing fresh-client-reference path; the old reference is not reused with a changed payload.

## Operator rules

- Pending drafts can be replayed with the existing Sync action.
- Blocked drafts require inspection and correction or explicit discard; they are not silently retried.
- Synced entries may be cleared from this device without deleting the server order.
- A draft whose product is no longer in the current cache may still be inspected; the UI does not invent a product name or price.
- All displayed cached prices are estimates, not a final sale total.

## Acceptance scenarios

1. Expand a pending draft and verify its business, branch, table, lines, attempt count and timestamps.
2. Inspect a draft containing a product absent from the current cache; the line should show an unknown product and unavailable price.
3. Inspect a table draft after its table has disappeared from the active cached list; the original table identifier remains visible.
4. Correct a blocked draft and verify that saving creates a new client reference.
5. Discard a blocked draft and clear a synced draft; verify IndexedDB errors are surfaced and no server order is deleted by local clearing.
6. Confirm indicative line values are labelled as cached estimates and are not used to assert the server's final order amount.

## External verification not performed here

Browser automation, real offline/online transitions, Go tests, PostgreSQL migrations/RLS, Next.js production build and authenticated staging acceptance remain external checks.
