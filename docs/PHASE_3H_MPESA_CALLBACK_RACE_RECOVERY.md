# Phase 3H — M-Pesa callback race recovery

## Scope

Migration `027_mpesa_callback_race_recovery.sql` closes the timing gap between
Safaricom returning STK request identifiers and the dispatch worker committing
those identifiers. A callback that arrives during that interval is validated
by the existing callback parser and durably quarantined in a restricted inbox.

## Behaviour

- Callback ingestion first attempts exact matching on both CheckoutRequestID
  and MerchantRequestID.
- If the attempt is not yet correlated, the callback fingerprint, both provider
  identifiers and JSON payload are stored in `mpesa_unmatched_callback_events`.
- The runtime database role has no direct table privileges on that inbox.
- Callback ingestion and dispatch completion both invoke the narrow
  `SECURITY DEFINER` correlation function. The dispatch worker also runs a
  periodic correlation pass as a final backstop for the narrow READ COMMITTED
  interleaving where each transaction could otherwise miss the other's
  uncommitted insert/update. This closes either transaction
  ordering: callback-before-dispatch-commit and dispatch-commit-before-callback.
- Correlation moves the event into the tenant-bound callback event table and
  advances the attempt to `callback_received` where appropriate.
- A successful callback arriving after an attempt has already become `failed`,
  `expired` or `review_required` does not silently resurrect it. It moves or
  keeps the attempt in `review_required` for provider reconciliation.
- Provider query handling now distinguishes known STK pending responses (`499`,
  `500.001.1001`), the reported expiry code (`1019`), and common terminal
  outcomes (`1`, `1032`, `1037`). Unknown non-zero result codes go to
  `review_required` rather than being guessed as failures. A late success callback
  after a terminal state also forces review.
- Callback ingestion still never verifies a payment, completes a sale or
  deducts inventory. Those remain the responsibility of provider reconciliation
  and atomic settlement.

## Operational safety

The unmatched inbox is not directly queryable by the runtime role. It contains
provider identifiers and callback payloads and must be treated as sensitive
payment metadata. It is intentionally retained for correlation and audit; a
separate retention policy must be approved before automated deletion is added.

No STK request is retried by this change. Ambiguous submissions remain in
`review_required`. This avoids duplicate customer prompts/charges.

## Verification status

The CI pipeline now applies 27 migrations and runs
`db/tests/mpesa_callback_race_recovery_smoke.sql`, which checks the inbox's
runtime-role isolation and verifies that both ingestion and dispatch completion
invoke correlation. Static formatting/configuration checks are performed in
this delivery environment. PostgreSQL execution and Go tests still require the
project's pinned Go 1.24.13 and PostgreSQL 17 CI environment.

A real Safaricom sandbox end-to-end test still requires configured Daraja
sandbox credentials, a callback-reachable HTTPS URL and a test handset. This
repository change does not claim that external provider test has been run.
