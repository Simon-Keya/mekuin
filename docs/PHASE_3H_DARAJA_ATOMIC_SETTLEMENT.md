# Phase 3H — Atomic Daraja Settlement (v53)

## Included

- `POST /api/v1/payments/mpesa/{attemptID}/settle`, protected by the authenticated tenant context and `pos.use` permission.
- The handler locks the attempt, sale and sale-payment row in one tenant transaction.
- Settlement is allowed only when the attempt is `provider_confirmed`, the sale is `pending_payment`, the tender is `pending_verification`, the receipt exists, and the attempt amount exactly equals the tender amount.
- This increment deliberately requires the single M-Pesa tender to equal the entire sale total. It does not infer mixed cash/M-Pesa split-tender policy.
- It reloads immutable sale lines and calls the existing recipe stock-consumption routine in the same transaction.
- It marks the payment verified, sale completed and attempt verified, and writes an audit event in that same transaction.
- A repeated call after successful settlement returns an idempotent success without writing stock movements again.
- Adds nullable `sales.source_order_id` for future pending order checkout orchestration. If populated, settlement transitions that open/held order to checked out atomically.

## Deliberate limitations

- v53 does not yet create pending M-Pesa sales or initiate STK Push from checkout. The existing checkout route remains cash-only and unchanged.
- Therefore this settlement endpoint is a guarded settlement primitive, not an end-to-end enabled M-Pesa checkout.
- The next increment must implement a pending-sale checkout path, create `sale_payments` with `pending_verification`, create the payment attempt, initiate STK Push with a durable retry/outbox strategy, and populate `source_order_id` for order checkout.
- Mixed tenders, multiple M-Pesa attempts, late success after expiry, cancellation, refunds to M-Pesa and reconciliation of provider-success-without-callback remain explicit follow-on cases.

## Migration 024

Adds `sales.source_order_id`, the FK/index, and narrowly scoped column update grants for settlement. It does not alter the existing cash checkout path.

## Verification

Static checks are performed when packaging. Go tests and PostgreSQL execution must be run with the repository's pinned Go 1.24.13 toolchain and PostgreSQL 17 before enabling this route in a live deployment.
