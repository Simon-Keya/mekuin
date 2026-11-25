# Phase 4C — Supplier invoice approval controls

## Scope
This increment adds an append-only invoice review-event stream, submission and decision endpoints, and a payment eligibility gate. Matching remains read-only; approval is a separate decision.

## Workflow
1. Finance staff record an invoice.
2. A user with `finance.manage` submits it for review. Rejected immutable invoice records are not resubmitted; record a corrected supplier document as a new invoice record using the supplier correction/reference process.
3. A user with `finance.approve` reviews the latest matching result and decides.
4. A clean match may be approved. A match with exceptions requires `approve_with_exceptions` and a reason. Rejection requires a reason.
5. The invoice creator cannot approve their own invoice.
6. Payment allocation re-reads the latest review event while holding the invoice row lock and only permits `approved` or `approved_with_exceptions`.

## API
- `POST /api/v1/payables/invoices/{invoiceID}/submit` — submit for review (`finance.manage`).
- `POST /api/v1/payables/invoices/{invoiceID}/review` — approve, accept exceptions, or reject (`finance.approve`).
- `GET /api/v1/payables/invoices/{invoiceID}/reviews` — immutable review history (`finance.read`).
- Invoice list and detail responses include `review_status`.

Decision body: `{ "decision": "approve|approve_with_exceptions|reject", "reason": "..." }`.

## Integrity and security
- Review events are append-only and tenant-scoped with forced RLS.
- The latest event is the authoritative workflow state using a per-invoice monotonic `event_no` (not transaction timestamps); no mutable invoice status column is repurposed.
- Review decisions lock the invoice row, serialize with other invoice workflow actions, and re-evaluate matching exceptions inside the transaction.
- Payment creation locks invoices and checks approval state in the same transaction as allocation creation.
- Manager and owner roles receive `finance.approve`; other roles do not.
- The creator/approver separation is enforced by the API.

## Verification limits
Go formatting and TSX syntax checks are source-level checks. Run migration 034 against a disposable PostgreSQL database, execute `go test ./...` with the pinned Go 1.24.13 toolchain, and run the Next.js build plus browser integration tests before deployment.
