# Phase 2 — Completion status

## Implemented in source
- [x] Product categories, units and directional unit conversions API and configuration UI.
- [x] Product catalogue UI for unit/liquid products, category, base unit, prices, reorder level and soft deactivation.
- [x] Recipe header + ingredient creation through existing tenant transaction; recipe cost and margin endpoint; recipe builder UI and live cost presentation.
- [x] Supplier list/create/update/deactivate API and directory UI.
- [x] Branch-scoped table list/create/update/deactivate API and table configuration UI.
- [x] Staff listing, invitation, role updates, profile creation API and staff UI.
- [x] Workspace links for the relevant operations screens.

## Behavioural safeguards
- No hard deletes for catalogue entities; deactivation preserves references/history.
- No client-side stock deduction in recipe management; sale workflow remains the stock movement source.
- Recipe cost is explicitly a live estimate based on current product catalogue cost price, not weighted-average stock valuation or a historical snapshot.
- Unit conversions are directional; reverse factors are not inferred.
- Staff PIN hashes are not returned to the UI. Manager PIN authorisation remains server-side.
- Tenant/branch scoping and permission checks remain enforced by existing API middleware and RLS.

## External acceptance still required
- Apply all migrations to a clean PostgreSQL 17 database and run the repository migration/RLS/privilege smoke tests.
- Run Go tests with the pinned Go 1.24.13 toolchain; the available local Go version in this environment is older.
- Install frontend dependencies and run Next.js production build/typecheck; `node_modules` is absent in this environment.
- Test each screen using real owner/manager/cashier/bartender/stock-controller accounts, including denied-permission cases.
- Verify invitation email delivery, invitation expiry/revocation/acceptance, branch assignments, and cross-tenant isolation.
- Validate recipe conversion errors and stock consumption against PostgreSQL using representative recipes.

Phase 2 is implementation-complete in source for the scope covered by the existing API. It is not production-accepted until the external checks above pass.
