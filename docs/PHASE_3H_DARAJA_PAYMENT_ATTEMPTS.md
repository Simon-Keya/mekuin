# Phase 3H — Daraja payment-attempt persistence (v49 WIP)

This increment builds on the v48 transport foundation. It adds a durable, tenant-scoped data model for correlating an M-Pesa STK attempt with a sale and tender, stores callback event payloads after correlation, and adds the STK Push Query transport operation.

## Added

- `mpesa_payment_attempts`: tenant/branch/sale/tender correlation, sandbox/production environment, phone, amount, provider IDs, status and receipt fields.
- `mpesa_callback_events`: durable JSONB event records with tenant-scoped idempotency fingerprint and processing metadata.
- `sales.pending_payment` lifecycle value reserved for the next checkout orchestration increment.
- `darajaClient.querySTK`: calls `/mpesa/stkpushquery/v1/query` using server-side OAuth and the shortcode/passkey/timestamp password.
- Unit tests exercise query payload, OAuth header, provider response correlation, and distinguish query acceptance (`ResponseCode`) from payment result (`ResultCode`).

## Safety boundary

This increment does **not** expose an M-Pesa checkout endpoint, does not register a callback route, and does not mark any payment verified. It does not alter v47 checkout behaviour. Existing checkout remains cash-only and continues to complete a sale and consume recipe stock in its transaction. Do not pass M-Pesa tenders through that path.

The `pending_payment` sale state and tables are schema groundwork only. Before enabling M-Pesa, the next increment must create pending sales/tenders, initiate STK Push with a retry-safe out-of-transaction provider call, ingest callbacks, correlate callback IDs to a stored attempt, perform an STK query, verify amount/phone/shortcode/receipt, and only then atomically complete the sale/order and consume recipe stock. A callback alone is not a trusted settlement signal.

## Important configuration note

The v48 provider client currently reads a single platform-level shortcode/passkey from environment variables. That is suitable only for a single configured merchant account. A multi-tenant production deployment needs tenant-owned merchant configuration and encrypted credential storage, or an explicit documented shared merchant arrangement, before production use.

## Migration

`021_mpesa_payment_attempts.sql` is additive and tenant-RLS protected. It grants runtime access only to the attempt/event fields needed by the future workflow. It does not grant the runtime role unrestricted mutation of payment records.
