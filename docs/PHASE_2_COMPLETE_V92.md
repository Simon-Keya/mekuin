# Phase 2 — Catalog and configuration completion (v92)

## Implemented

- Branch management: tenant-scoped branch list/create/update and soft activation state. No hard-delete operation is exposed.
- Staff: invitation, role, profile, manager PIN, and multi-branch assignment UI and API workflows.
- Product catalogue: unit/liquid products, category, base unit, prices and reorder metadata.
- Catalogue configuration: categories, units of measure and directional unit conversions.
- Recipes: output product, output quantity and ingredient rows; server-side recipe cost and gross-margin estimate.
- Suppliers: directory and maintenance.
- Tables: selected-branch table configuration.

## Security and data behaviour

- Branch handlers run behind authenticated tenant middleware and use the tenant transaction/business ID from server context.
- Branch creation and updates are owner-authorized through `staff.manage`; branch listing follows `staff.read`.
- Staff branch assignment validates UUIDs, uniqueness, business ownership and active branch status before replacing assignments.
- Existing branch records are deactivated, not deleted, preserving historical references.
- Manager PINs are hashed server-side and never read back into the UI.
- Recipe costing uses current `products.cost_price_minor` and directional unit conversions. It is not weighted-average inventory valuation or a historical cost snapshot; decimal minor-unit values are not rounded by the endpoint.

## Acceptance still required

- Go tests with the pinned toolchain.
- PostgreSQL 17 migrations, forced-RLS and cross-tenant negative tests.
- Next.js production build and browser-level workflow checks.
- Staging test with multiple branches and staff assignments, including an attempted cross-business assignment.

Implementation completion is distinct from environment acceptance. Do not deploy to production solely on the basis of this source package.
