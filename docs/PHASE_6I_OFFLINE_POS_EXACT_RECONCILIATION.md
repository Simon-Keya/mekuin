# Phase 6I — Exact offline POS order-reference reconciliation

## Problem
The previous client reconciled against the latest 100 open/held orders. Orders that were older or had moved to another status could be missed after a lost HTTP response.

## Implementation
`POST /api/v1/sales/orders/reconcile` accepts between 1 and 100 unique UUID `client_references`. It returns matching orders by exact reference, across all statuses, constrained to the authenticated business and branch and protected by `pos.use`.

The browser batches the references of pending/interrupted drafts into one lookup before replay. Matches are marked synced locally with their server order IDs. Unmatched drafts retain their original references and use the existing idempotent create endpoint.

## Safety
- No client-supplied tenant or branch IDs are accepted by the reconciliation endpoint.
- No order is created, edited, checked out, paid, or stock-deducted by reconciliation.
- The lookup is read-only and does not infer that an absent result guarantees the original request can never commit; stable server idempotency remains necessary.
- Maximum batch size is 100 to bound query work.

## External acceptance
1. Create an offline draft, allow the server to commit it, then simulate a lost response; reconciliation must find it.
2. Hold, reopen, cancel, or otherwise transition the order; exact lookup must still find it while it exists.
3. Create more than 100 newer orders; reconciliation must still find the older matching reference.
4. Switch business or branch; the old reference must not be exposed by the new scope.
5. Submit duplicate, malformed, empty, and over-100 reference batches; expect validation errors.
6. Submit an unmatched reference; client must retain the same reference for idempotent create retry.
