# Phase 3C — Recipe-backed stock consumption

## Delivered in source

- Checkout now reads active recipes for sold products and consumes their ingredients.
- Recipe ingredient quantities are converted into each ingredient product's base unit using the configured directional `unit_conversions` factor. A same-unit recipe ingredient uses factor 1.
- Multiple recipe lines referencing the same ingredient product are aggregated before a movement is written.
- A branch/product advisory transaction lock serializes concurrent balance checks and consumption.
- Current stock is calculated from the immutable `inventory_movements` ledger. Checkout rejects insufficient stock rather than allowing a negative balance.
- Required quantities must be positive and exactly representable to the ledger's eight decimal places; no silent rounding is applied.
- Each consumption is an immutable negative `sale_consumption` movement linked to the sale. The sale, sale lines, payment, idempotency record, audit entry and movements commit or roll back together.
- A unique partial index prevents duplicate consumption movements for the same sale/branch/product.

## Explicit limitations

- Only products with an active recipe are automatically deducted. Non-recipe products are not automatically tracked yet; the product stock-tracking flag and negative-stock policy remain unapproved.
- The implementation does not add a stock valuation method or change costing.
- VAT, service charge, discount order/rounding, M-Pesa/card verification, tabs/tables/held orders, split tender and refund restocking remain outstanding.
- Recipe output quantity is treated as the number of saleable outputs represented by one recipe batch. Confirm this interpretation against real operating recipes before production use.

## Verification status

- Migration 017 is included and CI migration expectation is updated to 17.
- Go unit/integration tests and PostgreSQL 17 execution must run in the user's environment; the assistant environment cannot download the pinned Go 1.24.13 toolchain or run PostgreSQL/Docker.
