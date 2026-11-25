# Phase 3H — Post-commit Daraja dispatch outbox (v55 WIP)

## What changed

- M-Pesa checkout now creates the pending sale, pending tender, `created` provider attempt and dispatch-outbox row in one tenant transaction.
- Checkout no longer calls Safaricom while holding the tenant database transaction open.
- A background worker claims only committed outbox rows through narrowly scoped `SECURITY DEFINER` functions.
- The worker submits STK Push after commit and records the provider request identifiers before the callback can be correlated.
- An ambiguous network outcome is moved to `review_required`; it is never automatically retried because Safaricom may already have accepted the original request.
- A worker claim that remains stale for more than two minutes is moved to `review_required`, not resent.
- The runtime role cannot directly read, update or delete dispatch-outbox rows; it can enqueue rows and execute the worker functions.
- Existing cash checkout remains unchanged. M-Pesa sales remain pending and recipe stock remains untouched until provider reconciliation and atomic settlement.

## Operational behavior

1. POS checkout commits sale, tender, attempt and dispatch row.
2. API responds `202 Accepted` with `provider_submission: queued_for_dispatch`.
3. Worker claims the committed row and sends one STK Push request.
4. On a valid accepted response, provider identifiers are persisted and attempt becomes `accepted`.
5. On timeout, transport error, invalid response or stale worker claim, the attempt becomes `review_required`. Operators must reconcile against Safaricom before any further action.
6. Callback ingestion, provider query reconciliation and atomic settlement remain separate stages.

## Safety boundary

- The worker does not mark a tender verified, complete the sale, deduct stock or release a POS table.
- No automatic resend occurs for ambiguous provider outcomes. A later operator-approved recovery workflow must first establish the provider-side state and must never blindly issue a second STK request.
- The worker currently uses the configured `DARAJA_ENV` and credentials for the process. Sandbox and production credentials must not be mixed; the stored attempt environment is checked against the configured base URL.
- The existing direct-sale-only limitation for M-Pesa remains; M-Pesa checkout from table/tab orders is still rejected.

## Verification status

Static formatting, JSON/YAML parsing, shell syntax and ZIP integrity are checked for this archive. Go tests and PostgreSQL 17 migrations/RLS smoke tests require the project's Go 1.24.13 and PostgreSQL CI environment and have not been executed in this workspace.
