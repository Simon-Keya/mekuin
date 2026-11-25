# Phase 4C — Supplier Payables Frontend

## Scope

Adds the `/payables` workspace page and a permission-aware workspace entry point. The page uses the authenticated API with cookie credentials and the selected tenant context.

## Included workflows

- Supplier invoice register with search, totals, allocated and outstanding amounts, and effective payment status.
- Record supplier invoices with supplier, invoice/due dates, currency, product lines, quantity, unit cost, computed line totals, supplier reference, and notes.
- Record supplier payments with payment method and external reference.
- Allocate a payment across eligible outstanding invoices for the same supplier and currency.
- Show unallocated payment balance after successful recording.
- Recent payment register.

## Accounting boundaries

The UI does not post a general-ledger journal, alter inventory, or claim three-way matching. The API remains authoritative for invoice arithmetic, supplier/product/order validation, payment allocation limits, and transaction atomicity. Currency amounts are entered in minor units (KES cents for KES), consistent with the API contract.

## Access

The workspace link is shown only when `finance.read` is present in the tenant context. API permissions remain enforced server-side.

## Verification limitations

The page was source-reviewed against the current payables endpoint request and response shapes. A Next.js production build, browser walkthrough, and live API/PostgreSQL verification have not been completed in this environment. Do not treat this increment as production-verified.
