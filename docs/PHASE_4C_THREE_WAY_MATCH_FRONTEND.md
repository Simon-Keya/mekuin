# Phase 4C — Three-way matching workspace

## Scope

This increment connects the accounts-payable register to the read-only invoice matching endpoint and adds purchase-order selection during invoice capture.

## User workflow

1. Open Accounts Payable and choose **Record invoice**.
2. Optionally select a purchase order. The workspace loads its product lines and links them to their purchase-order line IDs.
3. Review quantities and unit costs against the supplier invoice before recording it. The API continues to validate supplier ownership, product linkage, line totals and invoice totals.
4. In the invoice register, choose **Match** to retrieve a current comparison of invoice lines, ordered quantities/prices and posted receipt quantities/weighted costs.
5. Review each exception flag, including unlinked lines, no goods received, quantity overages and price variances.

## Control boundary

Matching remains read-only. This increment does not create approval states, release payment holds, post general-ledger entries, mutate inventory, or permit users to waive discrepancies. A match result is not an accounting approval.

## Permissions

The match view uses the existing `finance.read` protected route. Invoice creation and payment recording continue to use their existing API permissions and validations.

## Verification

- Reviewed the frontend API paths against the registered payables and purchasing routes.
- Checked the matching response fields against `getSupplierInvoiceMatch`.
- Production Next.js build and browser verification were not run because frontend dependencies are not installed in this workspace.
- PostgreSQL-backed matching behavior remains unverified in this environment.

## Next acceptance work

Before enabling approval, implement explicit exception review with actor, timestamp, reason, and immutable audit event; define who can approve clean matches versus waive exceptions; enforce approval and payment eligibility server-side; and add database integration tests for tenant boundaries and concurrent state changes.
