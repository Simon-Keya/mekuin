# Phase 2D — Catalogue configuration UI

## Implemented

- Added `/catalog/settings` for tenant-scoped product category, unit-of-measure, and unit-conversion administration.
- Category creation uses `POST /api/v1/catalog/categories`; deactivation uses the existing tenant-scoped deactivation endpoint. Category deletion is not exposed.
- Unit creation supports the API's `count`, `mass`, and `volume` dimensions. Unit deactivation preserves existing product/recipe references.
- Conversion creation uses `POST /api/v1/catalog/unit-conversions` with explicit source, target and positive decimal factor. Conversions are directional; the UI does not invent reverse factors.
- Existing conversion records are displayed read-only because no update/deactivation API is defined in this increment.
- Added navigation from `/products` to catalogue settings.

## Safety boundaries

- Tenant identity and permission enforcement remain server-side through the existing API middleware and `products.manage` / `products.read` permissions.
- The UI does not permit cross-dimension conversions itself; database constraints remain authoritative.
- The UI does not alter product base units, recipe ingredient units, stock balances, or historical movements.
- Deactivation is used instead of deletion to preserve historical references.

## Verification

Static source review only. External verification still required: install dependencies, run ESLint and the Next.js production build, execute Go tests with the pinned toolchain, apply migrations and smoke tests against PostgreSQL 17, and test category/unit/conversion workflows using authenticated tenant accounts.
