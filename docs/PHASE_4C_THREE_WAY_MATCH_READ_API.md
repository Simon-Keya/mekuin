# Phase 4C — Three-way invoice match read API

## Scope

Adds `GET /api/v1/payables/invoices/{invoiceID}/match` behind `finance.read`.
The endpoint is a read-only comparison of:

1. Supplier invoice line quantity and unit cost.
2. Linked purchase-order line ordered quantity and unit cost.
3. Posted receipt quantities and weighted-average receipt unit cost.

It does not approve invoices, create accounting entries, mutate receipts, or
change inventory. Existing invoices without a purchase-order link, or lines
without a valid linked purchase-order line, are returned as exceptions rather
than silently treated as matched.

## Response semantics

Each line returns `status` (`matched` or `exception`) and an `exceptions` list.
Possible exception codes:

- `unlinked_purchase_order_line`
- `no_goods_received`
- `invoice_quantity_exceeds_received`
- `invoice_quantity_exceeds_ordered`
- `purchase_order_price_variance`
- `receipt_cost_variance`

Receipt unit cost is calculated as the rounded weighted average of the posted
receipt line costs for the linked purchase-order line in the selected branch.
A line is considered matched only when it is linked and no listed exception is
present. Invoice quantities are compared as exact decimal rationals, not floats.

The top-level response includes invoice identity, purchase order reference,
line counts, and an aggregate `matched` or `exceptions` status.

## Controls and limitations

- The invoice lookup is scoped to the active business and branch.
- Every joined record is constrained by `business_id`; receipt aggregation is
  also constrained to the active branch and the invoice's purchase order.
- The endpoint is informational only. A future approval workflow must define
  tolerances, authorised overrides, and a durable approval event before it can
  block or release invoice posting.
- No currency conversion or tax calculation is inferred. Values remain in the
  invoice's recorded currency and integer minor units.
- This endpoint does not yet provide a user interface for selecting a PO or
  linking invoice lines; the invoice capture UI/API enhancement is a follow-up.

## Verification

Go formatting and source-level review are performed for this increment. Runtime
API tests require the repository's Go 1.24.13 toolchain and a PostgreSQL test
database; neither is claimed as completed by this change.
