# Phase 2 recipe, supplier and table UI increment (v91)

## Recipe builder

- Route: `/recipes`.
- Lists recipes and allows creation of a recipe linked to an active sellable output product.
- Supports output quantity, preparation notes, and multiple ingredient product/unit/quantity rows.
- Recipe creation calls the existing tenant-scoped API; the server writes the recipe and ingredients in one transaction.
- The detail view requests `/api/v1/catalog/recipes/{id}/cost` and presents the returned estimate.
- Cost basis is explicitly shown as current `products.cost_price_minor`. This is not stock valuation, WAC, or a historical sale snapshot. Directional unit conversions are required; missing conversions are reported by the server.

## Supplier directory

- Route: `/suppliers`.
- Uses existing tenant-scoped supplier list/create/update/deactivate endpoints.
- Deactivation preserves historical references.
- Does not invent supplier balances. Payables statements remain the source for derived balances.

## Table configuration

- Route: `/tables`.
- Uses active-branch table list/create/update/deactivate endpoints.
- Does not allow users to manually set occupancy or tab state; those are derived from POS orders.

## Workspace navigation

Added links from `/app` for recipes, tables, suppliers and owner staff administration.

## Verification limits

No database, Go 1.24.13 or dependency-installed Next.js environment was available for full runtime acceptance during this increment. Run the repository CI and authenticated PostgreSQL/staging checks before production use.

## Staff PIN form

The staff screen exposes one-time manager/owner PIN setting through the existing authenticated endpoint. The browser does not retrieve or persist the PIN; the API bcrypt-hashes it and returns only an update status.
