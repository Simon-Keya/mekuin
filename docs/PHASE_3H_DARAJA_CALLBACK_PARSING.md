# Phase 3H — Daraja callback parsing groundwork (v50 WIP)

This increment adds a strict parser for the documented STK Push callback envelope and deterministic event fingerprinting. It is deliberately a parsing/validation layer only; it does not expose a public callback route or mutate sales/payment state.

## Behaviour

- Rejects empty or oversized payloads and malformed JSON.
- Requires MerchantRequestID and CheckoutRequestID for correlation.
- Rejects duplicate or unnamed metadata keys.
- Extracts Amount, MpesaReceiptNumber, TransactionDate and PhoneNumber.
- Converts whole-KES callback amount into integer minor units with bounded numeric input.
- Requires amount, phone, receipt and transaction date on successful ResultCode=0 callbacks.
- Retains non-zero ResultCode as a failed/cancelled provider result; it never marks a payment successful.
- Produces a SHA-256 fingerprint of the exact raw payload for event idempotency. This is not a callback authenticity signature.

## Critical boundary

A callback is an untrusted provider notification, not settlement proof. A future handler must correlate both provider request IDs against a stored attempt, compare amount/phone/shortcode/currency against the attempt and sale, persist the raw event idempotently, and use the STK Push Query result before any payment is marked verified. The handler must then atomically finalize the sale and consume recipe stock exactly once. No such handler is enabled in v50.

## Tests

Unit tests cover successful metadata extraction, KES-to-minor-unit conversion, failed callback handling, missing success metadata, duplicate metadata and deterministic fingerprints. Runtime execution still requires the project's pinned Go 1.24.13 toolchain.
