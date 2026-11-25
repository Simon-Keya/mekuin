# Phase 2E — Recipes, costing, suppliers, tables and staff UI

## Delivered
- `/recipes`: creates recipes with output product, output quantity, instructions and multiple ingredient rows. Uses the existing transactional tenant API. Displays API-calculated cost, cost per output unit, gross margin and margin percentage.
- Costing is explicitly labelled as a live estimate using current `products.cost_price_minor` and directional unit conversions. It is not weighted-average inventory valuation or a historical sale snapshot. Missing directional conversions are surfaced by the API and must be configured in `/catalog/settings`.
- `/suppliers`: supplier directory with create, edit and soft deactivation; retained purchasing references are not deleted.
- `/tables`: branch-scoped table directory with create, edit and soft deactivation. Occupancy remains driven by POS/order lifecycle, not manually editable in this catalogue screen.
- `/staff`: invite team members for the selected branch, change membership roles and create staff profiles. The UI does not retrieve or display manager PINs.
- Workspace navigation links added according to the caller's permissions.

## Existing API contracts reused
- `/api/v1/catalog/recipes` GET/POST and `/recipes/{id}/cost` GET
- `/api/v1/catalog/suppliers` GET/POST/PATCH and supplier deactivate
- `/api/v1/catalog/tables` GET/POST/PATCH and table deactivate
- `/api/v1/staff/` GET, `/invitations` POST, `/{membershipID}/role` PUT, `/profiles` POST

## Important limits / acceptance
- Staff invitations are restricted by the API to cashier, bartender, stock_controller and manager roles. Existing owner membership is not granted by the invitation UI.
- Staff branch assignment in the invitation form is the currently selected branch only; multi-branch reassignment remains an explicit staff-management action/API.
- Staff profile creation is one-time where the database uniqueness constraint applies; profile editing is not exposed here.
- Manager PIN setting is not exposed in this screen. PIN hashes and lockout state remain server-side.
- Validate against PostgreSQL/RLS and authenticated roles in staging. No database or external service was exercised by this UI change.
