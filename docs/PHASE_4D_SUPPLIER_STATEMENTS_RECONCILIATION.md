# Phase 4D — Supplier Statements, Ageing & Payment Allocation Reconciliation

## Scope

This increment adds read-only accounts-payable reporting on top of the Phase 4C supplier invoice, payment, allocation and invoice-review records. It does not introduce a general ledger, journal posting, credit-note accounting, bank-feed import or provider settlement reconciliation.

## API

All endpoints require `finance.read` and execute in the selected tenant business/branch transaction:

- `GET /api/v1/payables/suppliers/{supplierID}/statement?currency=KES&from=YYYY-MM-DD&to=YYYY-MM-DD`
  - Currency is required because balances in different currencies must never be added together.
  - Optional `from` and `to` are inclusive. When `from` is supplied, the response includes an opening balance calculated from earlier transactions.
  - Supplier invoices are debits; supplier payments are credits on their payment dates.
  - Payment allocation and unapplied amounts are displayed separately. The statement does not imply that an unallocated amount has been matched to a particular invoice.
  - Void invoices are excluded. Other recorded invoices remain in the supplier subledger regardless of approval state; approval controls payment eligibility, not whether a recorded supplier invoice exists in the subledger.

- `GET /api/v1/payables/reports/aging?as_of=YYYY-MM-DD`
  - Groups outstanding invoice balances by supplier and currency.
  - Age is calculated from due date, falling back to invoice date where no due date exists.
  - Buckets: current (not yet due), 1–30, 31–60, 61–90 and over 90 days.
  - Outstanding is invoice total less recorded payment allocations, excluding void invoices.

- `GET /api/v1/payables/reports/reconciliation?from=YYYY-MM-DD&to=YYYY-MM-DD`
  - Shows each recorded supplier payment, its total allocations and its unallocated remainder.
  - Summary totals are grouped by currency. Cross-currency totals are intentionally not produced.
  - This is an internal payment-to-invoice allocation control, not bank statement or M-Pesa provider reconciliation.

## Accounting and data boundaries

- All amounts remain in the currency recorded on the source invoice/payment. No FX conversion is attempted.
- Payment records and allocations remain separate: the statement credits the full supplier payment and exposes any unallocated portion as a supplier-level credit awaiting allocation/review.
- The ageing report measures invoice outstanding balances from allocations; it does not net supplier-level unapplied credits against unrelated invoices.
- Reports are branch-scoped. A supplier may exist across a business, but this report includes only invoices and payments for the currently selected branch.
- No data is mutated by these report endpoints.

## Database support

Migration `035_payables_reporting_indexes.sql` adds branch/currency/due-date and branch/currency/payment-date indexes for the new read paths. Index effectiveness must be measured with representative production-like data using `EXPLAIN (ANALYZE, BUFFERS)` before making performance claims.

## Verification requirements

Before deployment:

1. Apply migrations 033–035 to a disposable PostgreSQL database.
2. Execute the supplier payables schema smoke checks and add DB integration coverage for branch isolation, date boundaries, void invoices, unapplied payments and multiple currencies.
3. Run `go test ./...` with the pinned Go 1.24.13 toolchain.
4. Run the Next.js production build and browser tests for the reports page.
5. Compare report totals against independently prepared supplier subledger fixtures.

The current source-level verification record is in `docs/PHASE_4D_VERIFICATION.md`.
