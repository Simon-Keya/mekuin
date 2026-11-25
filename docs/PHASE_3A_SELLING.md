# MEKU-In Phase 3A — Selling foundation

## Scope delivered

`POST /api/v1/sales/quote` is the first Phase 3 selling endpoint. It requires the authenticated tenant context and `pos.use` permission.

Request example:

```json
{
  "lines": [
    { "product_id": "<catalogue-product-uuid>", "quantity": 2 }
  ]
}
```

The server reads the current sale price, SKU, name and business currency from the tenant database. Client-supplied prices and totals are not accepted. Quantity is a positive whole number, duplicate product lines are rejected, and the basket is bounded to 100 lines. Amount multiplication and basket addition are checked for int64 overflow.

Response totals are integer minor units. `subtotal_minor` and `total_minor` are currently equal. Discount, tax and service-charge amounts are explicit zeroes until the business pricing policy is approved. The quote includes a `calculation_policy` explanation to prevent consumers from assuming tax or service charges were calculated.

## Tests added

- Pure Go tests for exact multiplication, aggregation, empty baskets, invalid quantities and overflow.
- Authenticated API smoke coverage using a catalogue product priced at KES 2,000.00; a quantity of two must return KES 4,000.00 (`400000` minor units).
- OpenAPI 3.1 route and request/response schemas at version 0.9.0.

## Deliberately not included yet

This increment is not checkout. It does not create orders, sales, payments or stock movements. Those must be implemented atomically in the next increments. It also does not invent VAT, service-charge, discount-order, rounding, negative-stock or payment-verification rules.

## Next increments

1. Confirm business pricing and rounding policy; extend quote calculations accordingly.
2. Add persisted order and checkout records with idempotency keys and immutable line snapshots.
3. Add payment lifecycle, keeping payment references distinct from provider verification.
4. Deduct recipe ingredients through immutable inventory movements in the checkout transaction, subject to the approved stock policy.
5. Add POS, Orders, Tables and Payments screens and acceptance scenarios.
