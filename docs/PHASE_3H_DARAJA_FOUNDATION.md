# Phase 3H — Daraja provider foundation (v48 work-in-progress)

This increment introduces an isolated Safaricom Daraja STK Push transport client and strict configuration validation. It is **not** a complete M-Pesa payment integration and must not be enabled for live checkout.

## Included

- Sandbox/production host selection (`DARAJA_ENV`), with optional HTTPS-only base URL override.
- Server-only consumer key, consumer secret, shortcode and passkey configuration.
- HTTPS-only callback URL validation.
- OAuth client-credentials token request.
- STK Push request construction using the shortcode/passkey/timestamp password.
- Phone and amount bounds; response acceptance requires `ResponseCode=0` and both Safaricom request IDs.
- Provider transport isolated from handlers for deterministic tests.

## Explicitly not included yet

- No checkout route invokes the provider client.
- No payment-attempt or callback persistence tables have been added.
- No callback handler, status-query worker, reconciliation job, callback authenticity control or duplicate-event processing exists.
- Existing v47 checkout still creates completed sales and consumes recipe stock in its transaction. Do not send M-Pesa tenders through it.
- No M-Pesa payment may be marked verified or represented as settled based on this foundation.

## Required next increment before sandbox checkout

1. Add durable payment attempts keyed by business/branch/sale/payment and unique `CheckoutRequestID`.
2. Change checkout orchestration so M-Pesa sales remain pending and do not consume stock or complete POS orders until verified.
3. Initiate STK Push outside the database transaction, then persist provider IDs with a retry-safe state transition.
4. Add public callback ingestion with bounded payloads, durable raw event storage, idempotent event keys and strict correlation to the stored attempt.
5. Confirm successful callback data with Daraja STK query before payment verification; match amount, shortcode/merchant, phone and receipt against the intended attempt.
6. On confirmed success, atomically mark the tender verified, complete the sale/order, apply recipe stock consumption and audit. On failure, retain the failed attempt and permit a new idempotent attempt.
7. Add timeout reconciliation for missing callbacks and concurrency tests for callback/query races.

## Provider protocol references

Safaricom's integration material documents the STK Push endpoint and the asynchronous callback flow; it also notes that callback delivery can fail and transaction-status query is the recovery path. See the Safaricom integration PDF linked in the project implementation notes. A synchronous `ResponseCode=0` is acceptance of the request, not proof of payment.

## v51 callback ingestion

`POST /daraja/stk/callback` is a public server-to-server endpoint. It accepts a
bounded STK callback body, validates its envelope and metadata, fingerprints the
exact payload, and invokes the narrowly scoped `ingest_mpesa_stk_callback`
SECURITY DEFINER function. The function resolves the exact stored attempt using
both CheckoutRequestID and MerchantRequestID, persists the callback under
forced tenant RLS, and ignores duplicate fingerprints. It may move an accepted
attempt to `callback_received` but never marks a tender verified, completes a
sale, or deducts stock. Unknown/mismatched provider identifiers receive a
non-disclosing acknowledgement; they do not alter payment state.

This is callback ingestion only. The callback endpoint is not a cryptographic
proof of origin and its payload is not settlement authority. A subsequent
STK Push Query/reconciliation worker must verify provider status and compare
amount, phone, merchant/checkout identifiers, and receipt before any settlement
transaction is implemented.
