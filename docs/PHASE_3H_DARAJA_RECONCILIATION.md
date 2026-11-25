# Phase 3H — Daraja provider reconciliation (v52)

## Included

- Authenticated, tenant- and branch-scoped `POST /api/v1/payments/mpesa/{attemptID}/reconcile`.
- Safaricom STK Push Query using the stored CheckoutRequestID.
- MerchantRequestID consistency check when returned by the provider.
- Persisted provider query timestamp, result code and description.
- A successful provider query is matched against the latest stored callback: both request identifiers, successful callback result, exact minor-unit amount, exact MSISDN and non-empty receipt must agree.
- Mismatches move the attempt to `review_required`; provider-declared failures move it to `failed`; incomplete provider results remain `verification_pending`.
- `provider_confirmed` is intentionally distinct from `verified`.

## Explicitly not included

This increment does not create STK payment attempts or initiate STK Push. It does not change checkout, complete sales, update sale payment status, deduct recipe stock, release POS tables, or settle refunds. A provider-confirmed attempt still requires a later atomic settlement transaction that locks the sale/attempt, validates the exact payment allocation and writes all recipe movements exactly once.

The callback body is a notification and is not cryptographically authenticated by this application. The provider query is correlated by CheckoutRequestID and MerchantRequestID, while callback metadata is used only as a consistency check. Unknown callback IDs remain intentionally undisclosed.

## Configuration

Uses the existing `DARAJA_ENV`, `DARAJA_BASE_URL`, `DARAJA_CONSUMER_KEY`, `DARAJA_CONSUMER_SECRET`, `DARAJA_SHORTCODE`, `DARAJA_PASSKEY`, and `DARAJA_CALLBACK_URL` configuration. Runtime secrets must remain in the deployment secret store.

## Verification status

Unit tests were authored for EAT transaction timestamp conversion. Static formatting and source-contract checks are separate from Go test execution and PostgreSQL migration execution; neither runtime suite is claimed as passed unless run in a Go 1.24.13 and PostgreSQL 17 environment.
