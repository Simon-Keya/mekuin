# Phase 4C — Supplier invoices and payables foundation

## Scope delivered

Migration `033_supplier_invoices_payables_foundation.sql` introduces tenant-scoped records for:

- Supplier invoices, including supplier invoice number, dates, currency snapshot, total, status, optional purchase-order link, and creator.
- Invoice lines, including product, description, quantity, unit cost, line total, and optional purchase-order-line reference.
- Supplier payments, recorded separately from goods receipts, with date, amount, currency, method, and external reference.
- Payment allocations connecting a supplier payment to an invoice.

All four tables enable and force row-level security using the established `app.business_id` tenant context. The runtime role receives read/insert permissions only; no general update or delete rights are granted. Indexes support branch/status invoice lists, supplier history, and invoice allocation lookups.

## Accounting boundaries

- A purchase receipt increases inventory through the existing inventory movement ledger. It is not itself a supplier payment.
- Creating an invoice or payment row does not post a general-ledger journal.
- Invoice `total_minor` and line `line_total_minor` are stored snapshots. The future write API must validate line totals and header totals in one transaction.
- Payment allocation amounts must not exceed the payment amount or the invoice's unpaid balance. This is a transactional API invariant and is not yet enforced by this schema-only increment.
- Payment and invoice currencies must match before allocation. Cross-currency settlement is not implemented.
- Supplier consistency between invoice, PO, PO line, payment and invoice allocation must be checked by the future API. The composite foreign keys enforce business ownership but do not by themselves guarantee same-supplier relationships.
- Invoice statuses are a data vocabulary only in this increment; no transition API has been enabled.

## Not delivered

No invoice endpoints, payment endpoints, approval workflow, receipt-to-invoice quantity matcher, credit notes, tax computation, journal posting, aging report, supplier statement, or frontend screens are included. No existing procurement or inventory behavior is changed.

## Verification

The migration was reviewed for PostgreSQL syntax and tenant-policy consistency against the existing procurement migrations. It has not been applied to a live/test PostgreSQL instance in this environment. Migration execution, RLS integration tests, and API tests remain required before runtime use.
