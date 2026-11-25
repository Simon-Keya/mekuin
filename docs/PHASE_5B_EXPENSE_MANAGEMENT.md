# Phase 5B — Expense management and cash integration

## Delivered

- Business-scoped expense categories, with a starter category set provisioned for existing businesses by migration and for newly registered businesses during workspace creation. Owners/managers can add categories.
- Branch-scoped expense register with date, payee, reference, notes, currency and integer minor-unit amount snapshots.
- New expenses enter `submitted`; approval/rejection is a separate manager/owner action. Rejection requires a reason. The creator cannot review their own expense.
- Append-only review decision history and append-only payment records.
- Approved expenses can be paid once using cash, bank transfer, mobile money, card or other.
- Cash payment requires the authenticated payer's active branch shift. The shift row is locked, available cash is checked, and the expense payment, drawer `paid_out` movement, expense status update and audit record commit in the same tenant transaction.
- Non-cash payments are recorded without a cash shift and do not affect expected drawer cash.
- Branch expense register and summary report grouped by category, currency and status, with optional date range.

## Lifecycle and accounting

`submitted → approved → paid`

`submitted → rejected`

An expense is not considered paid when it is merely approved. Payment is a distinct action and is only permitted after approval. The system records the full expense amount as a single payment; partial payments and split tender are not supported in this increment.

Cash drawer calculation remains:

`expected = opening float + cash sales + paid-in - paid-out`

For cash expenses, the paid-out movement is inserted automatically in the same transaction as the expense payment. Users must not create a second manual paid-out movement for that same expense. The payment row links to the shift for traceability.

All monetary values are integer minor units. Each expense retains its currency; this increment does not convert or aggregate unlike currencies into a single amount.

## Access controls

- `expenses.read`: list categories, expenses and summary report.
- `expenses.manage`: create expenses/categories and record payment after approval.
- `expenses.approve`: approve or reject submitted expenses.
- Owners retain wildcard access; managers have all three permissions. Cashiers, bartenders and stock controllers do not receive expense permissions by default.
- The API enforces permissions; frontend visibility is not an authorization boundary.
- RLS is enabled and forced on categories, expenses, review events and payment records. Review and payment history is append-only for the runtime role.

## Explicit boundaries

- Approval thresholds are not configurable yet; every expense requires approval.
- Creator self-approval is blocked. This also means a single-person owner workspace cannot approve its own submitted expenses; a second authorized reviewer is required.
- No partial payments, split tender, recurring expenses, attachments/receipt uploads, supplier invoice conversion, budget controls, tax/VAT treatment, reimbursements, cash refund handling or multi-currency conversion.
- Categories can be added but cannot yet be renamed/deactivated in the UI.
- Expense reports are branch-scoped and grouped by currency; no cross-branch consolidation is implied.

## Acceptance scenarios

1. Register a new workspace and confirm the starter categories exist; existing workspaces receive them through migration.
2. Create an expense and confirm it enters `submitted` and appears in the branch register.
3. Creator attempts to approve own expense: request is rejected. Another manager/owner can approve or reject; rejection without a reason is rejected.
4. Payment before approval is rejected; duplicate payment is rejected by state/unique constraints.
5. Cash payment without an open shift returns `cash_shift_required` and writes no payment, movement or status update.
6. Cash payment with an active shift creates one payment linked to the shift and one paid-out movement atomically. Insufficient drawer cash is rejected without partial writes.
7. Non-cash payment creates no drawer movement and does not change expected cash.
8. Another tenant/branch cannot read or mutate the expense. Runtime role cannot delete expenses or update/delete payment and review history.
9. Report totals are grouped by category, currency and status and honor optional date filters.

## Verification status

Source-level formatting, OpenAPI contract, workflow YAML, migration-count and frontend TSX syntax checks are run for this package. Go 1.24.13 tests/build, PostgreSQL 17 migration/RLS execution and Next.js lint/production build remain external CI gates when the pinned toolchain and services are available. Do not treat this source package as production-verified until those gates pass.
