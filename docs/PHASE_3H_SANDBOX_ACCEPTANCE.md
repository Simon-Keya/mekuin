# Phase 3H — Daraja sandbox acceptance and production gate

Status: **implementation WIP; sandbox acceptance not yet evidenced**.

This document is the repeatable acceptance procedure for the complete M-Pesa
STK Push lifecycle. Passing static checks or unit tests does not constitute a
successful Safaricom sandbox run or production approval.

## Required configuration

Configure these values only in the API hosting secret manager. Never commit
credentials, passkeys, callback payloads containing customer data, or live
provider secrets.

- `DARAJA_ENV=sandbox`
- `DARAJA_CONSUMER_KEY`
- `DARAJA_CONSUMER_SECRET`
- `DARAJA_SHORTCODE`
- `DARAJA_PASSKEY`
- `DARAJA_CALLBACK_URL` — public HTTPS URL ending in `/daraja/stk/callback`
- `DARAJA_BASE_URL` — leave unset unless Safaricom has supplied an approved
  HTTPS endpoint override

The callback URL must be reachable by Safaricom from outside the deployment.
Do not use localhost, a private network address, or a temporary tunnel as the
production callback endpoint. Use a stable HTTPS staging host for sandbox work.

## Automated checks

Run from the repository root with Go 1.24.13 and PostgreSQL 17 available:

```sh
cd services/api
go mod download
go test ./...
go test -race ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath ./cmd/api
cd ../..
```

Then run the complete CI workflow. It applies all migrations, provisions the
restricted runtime role, runs tenant/RLS and M-Pesa privilege smoke tests, and
runs the authenticated API smoke test. The CI PostgreSQL service is not a
substitute for a Safaricom sandbox transaction.

## Sandbox scenario matrix

Use a dedicated sandbox business and branch, a non-production test till and
Safaricom-provided test MSISDNs. Record attempt IDs and provider request IDs in
the test report; mask phone numbers and never store credentials in artifacts.

| Scenario | Expected outcome | Required evidence |
|---|---|---|
| Successful STK payment | Callback stored; query confirms; attempt becomes `provider_confirmed`; explicit settlement completes sale and deducts recipe stock once | Daraja response, callback event, query response, sale/payment state, movement count |
| Customer cancellation | Attempt becomes failed only after a recognized terminal provider result; sale remains unpaid; no stock movement | Provider result code and unchanged stock |
| Prompt timeout / unreachable phone | Pending until provider gives a recognized terminal result; no early failure | Query sequence and final state |
| Provider still processing | Remains `verification_pending`; no settlement | Query result code and repeated status snapshots |
| Duplicate callback | One logical event; no duplicate settlement or stock movement | Same callback replayed; event and movement counts |
| Callback before dispatch IDs commit | Callback quarantined then correlated after dispatch completion/backstop; no event loss | Inbox row and correlated callback event |
| Callback after terminal failure | Moves to manual review; never silently revives or settles | Attempt history and review queue |
| Provider success without callback metadata | Remains pending/review; no settlement | Query result and missing metadata evidence |
| Amount/phone/receipt mismatch | `review_required`; no settlement | Mismatch fixture and unchanged sale/stock |
| Duplicate checkout idempotency key | Same pending sale/attempt returned; no second STK prompt | Replayed request and single dispatch row |
| Dispatch timeout / lost response | `review_required`; never blindly sends another STK prompt | Worker logs, outbox state, provider-side lookup/review |
| Concurrent settlement requests | Exactly one transition and one set of stock movements; subsequent call is idempotent | Concurrent request results and movement counts |
| Insufficient recipe stock at settlement | Entire settlement transaction rolls back; tender remains unsettled | Error response and unchanged payment/sale/stock state |
| Cross-tenant / cross-branch access | No attempt details or mutation | Authenticated negative requests |

## Manual operational review

For `review_required` attempts:

1. Locate the attempt using the authenticated, branch-scoped operations queue.
2. Query Safaricom only when the stored CheckoutRequestID and MerchantRequestID
   are present and the configured environment matches the attempt environment.
3. Compare provider status and callback metadata against the stored amount,
   currency, phone and both request identifiers.
4. Record an investigation note and supporting provider reference without
   copying secrets or full customer phone numbers.
5. If evidence remains ambiguous, leave the attempt in review. Do not issue a
   second STK Push for the same pending sale and do not force settlement.
6. Escalate discrepancies to the merchant's Safaricom support channel and
   reconcile against the merchant statement before any manual accounting
   adjustment. Any adjustment must be a separate audited workflow.

## Go-live gate — all must pass

- [ ] Go 1.24.13 unit, race, vet and build checks pass in CI.
- [ ] PostgreSQL 17 migration and all RLS/privilege smoke tests pass.
- [ ] Sandbox success, cancellation, timeout, duplicate, early callback,
      ambiguous dispatch and concurrent settlement scenarios pass.
- [ ] Sandbox report includes redacted request/response evidence and database
      before/after assertions.
- [ ] Merchant confirms shortcode, transaction type, callback URL and settlement
      account configuration with Safaricom.
- [ ] Production credentials are stored in the secret manager and isolated from
      sandbox credentials.
- [ ] Production callback endpoint is deployed over HTTPS and externally
      reachable.
- [ ] Operator access, review queue ownership, escalation and reconciliation
      procedures are trained and documented.
- [ ] A controlled low-value production pilot is approved by the merchant and
      MEKU Solutions, with rollback/disable procedure tested.

Until every item is checked with retained evidence, keep `DARAJA_ENV=sandbox`
and do not advertise M-Pesa as production-ready. Cash checkout remains the
existing supported fallback.

## Provider result-code policy

The application treats only a small set of known STK outcomes as terminal.
Known in-progress/session codes remain pending; unknown codes require manual
review. This is deliberately conservative because result-code meanings can
vary by API and provider environment. Confirm observed codes against the
merchant's current Safaricom Daraja documentation and actual sandbox responses
before approving go-live. A synchronous STK Push acceptance response is not a
payment confirmation.

## Local sandbox configuration preflight

Before starting the API in a dedicated sandbox environment, run:

```sh
set -a
. ./.env
set +a
sh scripts/mpesa-sandbox-preflight.sh
```

The script requires `DARAJA_ENV=sandbox`, checks that the required secrets are
present without printing them, enforces an HTTPS callback ending in
`/daraja/stk/callback`, and rejects a non-standard `DARAJA_BASE_URL` override.
It does not contact Safaricom, verify DNS/TLS reachability from Safaricom, or
make a payment. Those checks remain part of the live sandbox acceptance run.

The Go provider contract tests use an in-process HTTP test server to verify the
OAuth request, STK Push payload fields, timestamp/password construction,
response correlation IDs and local input validation. They are deterministic
transport tests, not substitutes for provider sandbox acceptance.
