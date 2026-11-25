# Phase 2B — Catalog API increment

All endpoints below require an authenticated session and a selected, active business/branch workspace. The API derives tenant identity from the session and membership validation. Do not send `business_id` or `branch_id` to catalog write endpoints.

Base path: `/api/v1/catalog`

| Method | Path | Required permission | Purpose |
|---|---|---|---|
| GET | `/categories` | `products.read` | List business categories |
| POST | `/categories` | `products.manage` | Create category |
| GET | `/units` | `products.read` | List units of measure |
| POST | `/units` | `products.manage` | Create count, mass or volume unit |
| GET | `/unit-conversions` | `products.read` | List same-dimension unit conversions |
| POST | `/unit-conversions` | `products.manage` | Create a directed unit conversion factor |
| GET | `/products` | `products.read` | List products |
| POST | `/products` | `products.manage` | Create unit or liquid product |
| GET | `/suppliers` | `suppliers.read` | List suppliers |
| POST | `/suppliers` | `suppliers.manage` | Create supplier |
| GET | `/tables` | `tables.manage` | List selected branch tables |
| POST | `/tables` | `tables.manage` | Create selected branch table |
| GET | `/recipes` | `products.read` | List recipe headers |
| POST | `/recipes` | `products.manage` | Create recipe and ingredients atomically |

## Request examples

Category:

```json
{"name":"Spirits","description":"Distilled beverages"}
```

Unit:

```json
{"code":"ML","name":"Millilitre","dimension":"volume"}
```

Unit conversion:

```json
{"from_unit_id":"<litre-unit-uuid>","to_unit_id":"<millilitre-unit-uuid>","factor":"1000"}
```

The factor is directed: one unit of `from_unit_id` equals `factor` units of `to_unit_id`. The database rejects non-positive factors and conversions across different dimensions. A reverse conversion is a separate row; no reciprocal is silently generated.

Unit product:

```json
{"base_unit_id":"<unit-uuid>","sku":"MIXER-001","name":"Tonic Water","product_kind":"unit","cost_price_minor":12000,"sale_price_minor":18000,"reorder_level":"12","is_sellable":true}
```

Liquid product:

```json
{"base_unit_id":"<volume-unit-uuid>","sku":"GIN-750","name":"House Gin 750ml","product_kind":"liquid","cost_price_minor":180000,"sale_price_minor":0,"reorder_level":"1","bottle_size":"750","serving_size":"30","is_sellable":false}
```

Recipe:

```json
{"output_product_id":"<sellable-product-uuid>","name":"Gin and Tonic","output_quantity":"1","ingredients":[{"ingredient_product_id":"<gin-product-uuid>","unit_id":"<ml-unit-uuid>","quantity":"30"},{"ingredient_product_id":"<tonic-product-uuid>","unit_id":"<ml-unit-uuid>","quantity":"150"}]}
```

Quantities are decimal strings to avoid binary floating-point conversion. Monetary values are integer minor units. The liquid product's `base_unit_id` must use the volume dimension. Recipe ingredient units must match the ingredient product's dimension. Recipe costing is not yet exposed and must not be inferred from these endpoints.

## Error responses

- `400`: malformed JSON or invalid basic request shape.
- `403`: missing role permission or tenant access.
- `409`: tenant-scoped duplicate SKU/name/code.
- `422`: database constraint or referenced-record validation failure.
- `503`: tenant transaction or commit failure.
- `500`: unexpected catalog operation failure.

The request transaction is rolled back whenever the handler returns a 4xx/5xx response. Recipe header and ingredient inserts therefore succeed or fail together.

## Remaining Phase 2B work

Staff profiles, role/branch assignment, manager PIN lifecycle, unit conversion update/deactivate flows, catalog update/deactivate flows, recipe costing/margin service, and PostgreSQL-backed API integration tests remain to be implemented.

## Catalog maintenance increment

Catalog records are maintained through updates and deactivation rather than hard deletion. Added `PATCH /api/v1/catalog/categories/{id}`, `PATCH /api/v1/catalog/products/{id}`, `PATCH /api/v1/catalog/suppliers/{id}`, and `PATCH /api/v1/catalog/tables/{id}`. Added `POST .../{id}/deactivate` for categories, units, products, suppliers, and tables. Product updates intentionally leave SKU, base unit, and product kind immutable in this endpoint; changing those foundational attributes needs a separately reviewed migration workflow. Table updates are scoped to the authenticated selected branch. Supplier directory reads use `suppliers.read`, granted to managers and stock controllers; supplier writes remain `suppliers.manage`.

These routes are source-implemented. PostgreSQL-backed execution is still pending; see `PHASE_2_STATUS.md` for the exact acceptance state and CI evidence requirements.


## Staff API increment

Base path: `/api/v1/staff`. Staff reads require `staff.read` (owner and manager). Profile creation, branch assignment, role changes, and manager PIN updates require `staff.manage` (owner only). All operations are tenant-scoped through the authenticated workspace transaction.

| Method | Path | Permission | Purpose |
|---|---|---|---|
| GET | `/` | `staff.read` | List memberships, user identity, optional profile and branch assignments |
| POST | `/memberships` | `staff.manage` | Add an existing registered account to this business and assign branches atomically |
| POST | `/profiles` | `staff.manage` | Create a staff profile for an existing membership |
| PUT | `/{membershipID}/branches` | `staff.manage` | Replace branch assignments |
| PUT | `/{membershipID}/role` | `staff.manage` | Change membership role; cannot demote the last active owner |
| PUT | `/{membershipID}/manager-pin` | `staff.manage` | Set a 4–8 digit PIN for an owner/manager profile; only bcrypt hash is stored |

Profile creation body: `{"membership_id":"<membership-uuid>","display_name":"Jane Doe","phone":"+254..."}`. Branch assignment body: `{"branch_ids":["<branch-uuid>"]}`. Role body: `{"role":"cashier"}`. PIN body: `{"pin":"1234"}`. The membership provisioning endpoint requires the user to register an account first; email invitation delivery, invitation acceptance, self-service PIN change and membership suspension remain unimplemented.

## Recipe cost estimate

`GET /api/v1/catalog/recipes/{id}/cost` returns a live estimate from the
current `products.cost_price_minor` values. Ingredient quantities are converted
from the recipe ingredient unit to the ingredient product's base unit using a
directional `unit_conversions` row (`from_unit_id` -> `to_unit_id`). Identical
units require no conversion. Missing directional conversions return HTTP 422;
they are never treated as zero-cost ingredients.

The response includes total recipe cost, cost per output unit, gross margin
against the output product's current sale price, and the cost basis. Values
are decimal minor-unit strings and are not rounded. This is deliberately not
weighted-average costing, stock valuation, a tax calculation, or a historical
sale snapshot. It changes when current catalog cost or sale prices change.

## OpenAPI contract

The API publishes an OpenAPI 3.1 contract at `GET /openapi.json` (v0.4.0). It includes cookie authentication, a shared structured error schema, and detailed request/response schemas for registration, workspace selection, staff membership/profile, manager authorization, units, conversions, products and recipes. Additional schemas are still required for every remaining catalog maintenance operation and foundation response.

## Staff role concurrency

Role changes lock the tenant business row before checking the active-owner count and updating a membership. This serializes concurrent role changes for a business and prevents two requests from independently demoting the final owners.

### Collection pagination and search (v22)

All catalog collection GET endpoints and the staff directory accept `limit`, `offset`, and `q`:

- `limit`: defaults to 50; accepted range 1–100.
- `offset`: defaults to 0; accepted range 0–1,000,000.
- `q`: optional trimmed search text, maximum 100 characters.
- Invalid bounds return HTTP 400 (`invalid_pagination_or_search`).
- Results use stable ordering and parameterized SQL. Search is applied to the relevant display/name/code fields; tables remain scoped to the selected branch.

These parameters apply to categories, units, unit conversions, products, suppliers, tables, recipes, and staff. Pagination is offset-based; cursor pagination is not currently implemented.

### Manager action authorization

`POST /api/v1/staff/authorize-action` accepts `manager_membership_id`, `pin`,
`action`, and optional `target_id`. It issues a two-minute single-use approval token
only after verifying a different active owner/manager. Allowed actions are
`discount.approve`, `sales.void`, `sales.refund`, and `inventory.count.approve`.
Five failed attempts in a 15-minute window lock the approver PIN for 15 minutes.
Sensitive operation handlers must call `consumeManagerApproval` within their own
business transaction; the token is scoped to requester, business, branch, action,
and target. Phase 2 does not yet contain the downstream sale/refund/count handlers.

### Manager action authorization (Phase 2H hardening)

`POST /api/v1/staff/authorize-action` accepts `manager_membership_id`, `pin`,
`action`, and optional `target_id`. The manager membership must belong to the
selected business, be active, have an active staff profile and a configured PIN,
and belong to a different user from the requester. The allowlist is limited to
`discount.approve`, `sales.void`, `sales.refund`, and
`inventory.count.approve`.

A successful check returns a random, two-minute, single-use token. Only its
SHA-256 digest is stored. The token is scoped to requester, business, branch,
action and optional target. Failed PIN checks return HTTP 200 with
`authorized: false` so the tenant transaction commits the failed-attempt counter
and audit event; clients must check `authorized`, not just the HTTP status. Five
failures in a 15-minute window lock the PIN for 15 minutes.

Sensitive operation handlers must consume the token through
`consumeManagerApproval` in the same database transaction as the operation.
Issuing a token alone does not authorize an operation. This primitive is not yet
wired to sales, refunds or stock-count endpoints because those operational
modules are not yet implemented.
