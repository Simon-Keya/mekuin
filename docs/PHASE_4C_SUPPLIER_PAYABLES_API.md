# Phase 4C — Supplier Payables API

## Endpoints

All routes are under the authenticated, selected-tenant API context.

| Method | Route | Permission | Purpose |
|---|---|---|---|
| GET | `/api/v1/payables/invoices` | `finance.read` | Branch-scoped invoice list with allocated and outstanding balances |
| POST | `/api/v1/payables/invoices` | `finance.manage` | Record an invoice and its lines atomically |
| GET | `/api/v1/payables/invoices/{invoiceID}` | `finance.read` | Invoice header, lines, and computed balance |
| GET | `/api/v1/payables/payments` | `finance.read` | Branch-scoped supplier payment history |
| POST | `/api/v1/payables/payments` | `finance.manage` | Record a payment and its allocations atomically |

Managers receive `finance.read` and `finance.manage`; owners retain all permissions. Other roles do not receive these permissions by default.

## Write rules

- Invoice header total must equal the sum of submitted line totals.
- Each line total must equal PostgreSQL `round(quantity * unit_cost_minor)`.
- A linked purchase-order line must belong to the selected purchase order and the same product.
- If a purchase order is supplied, it must belong to the selected branch and the same supplier.
- Payment allocations must be positive, unique per invoice in a request, sum to no more than the payment amount, and match the invoice supplier and currency.
- Invoice rows are locked in sorted UUID order before outstanding balances are checked. This serializes concurrent allocation attempts against the same invoice and avoids inconsistent over-allocation.
- Payment header and all allocations are inserted in one tenant transaction. A failure rolls back the entire operation.
- Invoice `paid` and `partially_paid` states are computed from allocations at read time. The stored status remains the immutable `recorded` value in this increment.

## Request shapes

Invoice request:

```json
{
  "supplier_id": "<uuid>",
  "purchase_order_id": "<optional uuid>",
  "invoice_number": "SUP-INV-1001",
  "invoice_date": "2026-11-24",
  "due_date": "2026-12-24",
  "currency": "KES",
  "total_minor": 150000,
  "lines": [
    {
      "purchase_order_line_id": "<optional uuid>",
      "product_id": "<uuid>",
      "description": "Product description",
      "quantity": "2",
      "unit_cost_minor": 75000,
      "line_total_minor": 150000
    }
  ]
}
```

Payment request:

```json
{
  "supplier_id": "<uuid>",
  "payment_reference": "PAY-2026-0001",
  "payment_date": "2026-11-24",
  "amount_minor": 100000,
  "currency": "KES",
  "payment_method": "bank_transfer",
  "allocations": [
    { "invoice_id": "<uuid>", "amount_minor": 100000 }
  ]
}
```

`amount_minor` uses the currency's minor unit (for KES, cents). A payment may contain an unallocated remainder, returned explicitly as `unallocated_minor`.

## Not yet included

No invoice editing, voiding, credit notes, receipt-quantity matching, invoice approval, supplier statements, aging report, tax engine, or general-ledger journal posting is included. The current PO link validates supplier and product identity, not three-way quantity/price matching against receipt records. Runtime integration tests against PostgreSQL remain required.
