# Phase 3H — Pending M-Pesa POS checkout (v54 WIP)

## Implemented

- The existing `POST /api/v1/sales/checkout` accepts a single full-value `mpesa` tender in addition to the existing cash flow.
- M-Pesa tender requests require `phone_e164` in Kenyan international format (`254` plus nine digits).
- The tender amount must be a whole-KES amount (minor units divisible by 100) and within the Daraja STK Push maximum of KES 250,000.
- The server continues to reload product prices and calculate totals. Client totals and payment references are not trusted.
- The checkout transaction creates the sale with `pending_payment`, immutable sale-line snapshots, a `pending_verification` sale payment, and a durable `created` M-Pesa attempt before requesting STK Push.
- A successfully accepted STK request stores both provider request identifiers and moves the attempt to `accepted`.
- Recipe-backed stock consumption is deferred for M-Pesa and remains part of the existing atomic settlement endpoint only.
- Cash checkout and cash split tender behavior remain unchanged.
- Idempotency is stored for the M-Pesa sale as well as cash sales.

## Deliberate limits

- M-Pesa must be one tender for the entire sale. Mixed cash/M-Pesa, multiple M-Pesa attempts, and partial provider allocations are rejected.
- M-Pesa checkout from a table/tab order is rejected in this increment. The pending-order lifecycle needs its own explicit status and table-occupancy behavior.
- Card checkout remains disabled.
- An STK network error is an ambiguous provider outcome. The attempt and pending sale are retained and the API responds `202` with `outcome_unknown_requires_review`; clients must not create a new sale or blindly submit a second STK request.
- The current HTTP tenant middleware holds the tenant transaction open while the synchronous STK call is made. A provider callback that arrives before the transaction commits can be acknowledged as unknown by the callback correlation function. This race must be removed with a durable post-commit dispatch/outbox before production enablement.
- The ambiguous `created` attempt has no automated safe retry in this increment. Operator review is required; do not infer failure or initiate a duplicate charge.

## Settlement sequence

1. POS submits a single M-Pesa tender with `phone_e164` and an idempotency key.
2. Server creates pending sale, tender and attempt, then requests STK Push.
3. Customer completes or cancels the STK prompt.
4. Callback is ingested as correlation evidence only.
5. Authenticated operator invokes provider reconciliation.
6. Only a matching provider success plus callback metadata produces `provider_confirmed`.
7. The atomic settlement endpoint verifies the payment, completes the sale, deducts recipe-backed stock, closes a linked order where supported, and writes audit data in one transaction.

## Not production-ready

This increment is a WIP integration, not a production Daraja certification. The pre-commit callback race, missing automated safe retry for ambiguous submissions, live Safaricom credentials/callback validation, and end-to-end PostgreSQL tests remain outstanding. Do not enable production payments until those are addressed and sandbox/live certification is completed.

## v55 correction

The v54 implementation called Safaricom from inside the checkout transaction. v55 replaces that behavior with a durable post-commit dispatch outbox. See `PHASE_3H_DARAJA_POST_COMMIT_DISPATCH.md`. M-Pesa table/tab order checkout remains unsupported.
