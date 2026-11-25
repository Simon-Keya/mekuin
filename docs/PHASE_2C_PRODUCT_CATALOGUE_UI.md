# Phase 2C — Product catalogue UI increment

## Implemented

- Added `/products`, a tenant-authenticated product catalogue screen using the existing `/api/v1/catalog/products`, `/categories`, and `/units` endpoints.
- Added search by product name/SKU and filters for unit and liquid products.
- Added product creation with SKU, name, category, base unit, cost and sale price, reorder level, sellable state, and optional liquid bottle/serving sizes.
- KES values are converted to integer minor units before submission; quantities remain decimal strings for PostgreSQL NUMERIC.
- Added soft deactivation through the existing tenant-scoped endpoint; historical records are retained.
- Added permission-aware navigation from the operations workspace.

## Explicit boundaries

- The API remains authoritative for tenant, permission, unit-dimension, and liquid-size validation.
- This screen does not introduce stock balances or infer on-hand quantities from the product catalogue.
- Editing is not exposed in this increment. The API supports a restricted update contract, but a safe edit workflow needs to load the full current product state and preserve immutable SKU/base-unit/product-kind fields.
- Categories and units must already exist. Their administration UI is a separate catalogue configuration increment.

## Verification

Static source review only in this environment. Run the web lint and production build, then authenticate with a user holding `products.read` / `products.manage` against PostgreSQL-backed staging. Verify unit and liquid creation, invalid dimension rejection, duplicate SKU handling, and soft deactivation.
