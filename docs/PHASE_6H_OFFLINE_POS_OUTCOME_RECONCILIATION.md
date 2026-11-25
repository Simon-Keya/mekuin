# Phase 6H — Offline POS outcome reconciliation

## Delivered
- Branch-scoped POS order-list rows now include `client_reference`.
- Before replaying pending or interrupted local drafts, the web POS fetches the authenticated branch's open/held orders and matches exact client references.
- A matching order marks the local draft `synced` and stores its server order ID without issuing another create request.
- Business/branch scope is checked before accepting a reconciliation match.
- Unmatched drafts retain the same reference and follow the existing idempotent create path.

## Safety and limitations
- The order list currently returns only the latest 100 open/held orders. A missing match is therefore inconclusive, not evidence of a failed server write.
- Retrying an unmatched draft remains safe only because the server create endpoint locks and checks the same tenant/branch-scoped client reference and canonical payload hash.
- If a draft's original order is no longer open/held, it may not appear in the list; the idempotent POST response remains the fallback confirmation path.
- No offline checkout, payment, revenue recognition, cash movement or stock deduction is introduced.

## External acceptance
1. Simulate server commit followed by dropped HTTP response; reconnect and sync; confirm list reconciliation marks the draft synced.
2. Confirm no second order is created.
3. Confirm another branch cannot reconcile the draft.
4. Confirm a missing list match retries with the unchanged client reference.
5. Confirm existing order-list consumers tolerate the additional response field.

Go/PostgreSQL/Next.js/browser integration checks remain deployment-side acceptance tasks.
