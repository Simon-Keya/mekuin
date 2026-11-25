# Phase 2 completion tracker

Status: IN PROGRESS — not accepted as complete.

- [x] Tenant-scoped catalog schema (migration 008)
- [x] Category/unit/product/supplier/table/recipe list + create APIs
- [x] Directional unit conversion list + create APIs
- [x] Catalog update/deactivation APIs (v17 increment)
- [x] Staff profile creation and tenant-scoped directory read
- [x] Existing-account staff membership provisioning with branch assignment and audit
- [x] Role changes and branch assignment APIs with last-owner protection
- [x] Manager PIN set (bcrypt) and audit
- [x] Manager PIN verification, action-bound approvals and failed-attempt lockout primitive (self-service PIN change remains unimplemented)
- [x] Recipe cost and margin output with explicit, documented current catalog cost basis (no invented inventory valuation)
- [x] OpenAPI 3.1 route contract published at `/openapi.json` with common error schema and core Phase 2 request/response schemas (full route-by-route schema coverage remains)
- [x] Bounded search/pagination for catalog collections and staff directory (source-level; database runtime verification pending)
- [ ] PostgreSQL migration execution and tenant/RLS smoke tests
- [x] API integration smoke coverage for catalog, staff membership lifecycle, manager PIN success/lockout, recipe costing, unit conversion and cross-tenant denial (execution pending)
- [ ] API integration coverage for approval consumption by sensitive HTTP operations and comprehensive permission/rollback paths
- [ ] Full Phase 2 acceptance walkthrough


- [x] Last-active-owner protection serialized with a business-row lock to prevent concurrent demotions.

Do not mark Phase 2 complete until every unchecked item is either implemented and tested or explicitly recorded as deferred by an approved product decision.

### Phase 2F follow-up — collection coverage

Pagination/search now covers every catalog collection endpoint as well as the staff directory. Bounds are shared (`limit` 1–100, default 50; `offset` 0–1,000,000; `q` max 100 characters). This is source-level implementation; runtime SQL and PostgreSQL integration verification remain outstanding.

## Phase 2H — manager PIN authorization (v23)

Implemented a dedicated authorization endpoint at `POST /api/v1/staff/authorize-action`.
A request names the manager membership whose PIN is being verified. The approver must
be an active owner/manager with an active staff profile and must be a different user
from the requester. PINs remain bcrypt hashes. Failed attempts are counted in a
15-minute window; five failures lock that manager PIN for 15 minutes. Successful
verification resets the failure window and issues a cryptographically random token
whose SHA-256 digest is stored in PostgreSQL. The token expires after two minutes and
is bound to business, selected branch, requester, action, approver and optional target.

The `consumeManagerApproval` helper atomically consumes a matching token and must be
called inside the same transaction as the sensitive operation. Current allowed action
keys are `discount.approve`, `sales.void`, `sales.refund`, and
`inventory.count.approve`. No sales/stock action handlers exist yet in Phase 2, so
this is an authorization primitive, not evidence that later operational handlers
already enforce it. Do not release those handlers without calling the consume helper.

Migration `009_manager_pin_authorizations.sql` adds attempt state and the tenant-RLS
approval table. Migration execution and Go/PostgreSQL tests remain pending in this
execution environment.

### Phase 2I follow-up — branch-scoped manager approval

Manager PIN verification now additionally requires the approving manager/owner to have an active membership-to-branch assignment for the currently selected branch. A manager with a valid business-level PIN but no assignment to that branch cannot approve an action there. This check is performed in the same tenant transaction as PIN verification. Runtime PostgreSQL verification remains pending.

## Phase 2J — manager authorization database smoke coverage

Added `db/tests/manager_authorization_rls_smoke.sql`. Run it as the migration/database owner after all Goose migrations. It verifies that the runtime role sees only current-tenant approvals, cannot insert another tenant's approval, cannot bind an approval to a branch from another tenant, and cannot persist an action outside the authorization allowlist. The script rolls back all fixtures. This test has been authored but has not yet been executed against PostgreSQL in this environment.

### Phase 2K follow-up — approval lifecycle database assertions (v27)

The manager authorization smoke test now additionally exercises a matching approval
being consumed once, replay rejection, expiry rejection, action mismatch rejection,
and tenant-scoped persistence of failed-PIN counter state. These are SQL assertions
for execution against the migrated PostgreSQL database; they have not been executed
in this environment because neither `psql` nor Docker is installed here.

The SQL test validates database behavior, not the HTTP handler's transaction wrapper
or full end-to-end manager PIN flow. Those still require API integration tests.


### Phase 2L follow-up — authorization input hardening (v28)

The manager authorization handler now validates manager membership IDs and optional target IDs as canonical UUIDs before issuing SQL, and validates the PIN as 4–8 numeric digits through a shared pure helper. This prevents malformed UUID input from surfacing as a database error and gives the request-validation tests a direct target. The helper tests are authored but remain unexecuted until the required Go toolchain is available.


### Phase 2M — CI database smoke-test coverage (v29)

The PostgreSQL CI job now executes all three database smoke suites after migrations and runtime-role provisioning: foundation tenant isolation, catalog RLS/constraints, and manager-authorization RLS/lifecycle assertions. Matching Make targets are available: `make db-rls-smoke`, `make db-catalog-smoke`, `make db-manager-auth-smoke`, and `make db-all-smoke`. This wires the checks into CI; it is not evidence that the hosted CI workflow has run successfully.


### Phase 2M follow-up — approval action-binding smoke coverage

The manager-authorization SQL smoke test now uses a separate, unexpired approval
fixture to prove that an approval cannot be consumed under a different action,
while confirming that the same fixture remains consumable under its bound action.
The expired-token case remains independently tested. This is test coverage only;
PostgreSQL execution is still required in CI before acceptance.

### Phase 2N follow-up — approval row privilege hardening

Migration `010_manager_approval_column_privileges.sql` narrows the runtime role on
`manager_action_approvals`: it can insert valid approval records, read only the
columns required for atomic consumption, and update only `consumed_at`. It cannot
rewrite the action, tenant/branch/requester scope, token digest, or expiry after
issuance. The manager authorization smoke test includes negative privilege checks.
This is source-level coverage only until migrations and SQL smoke tests execute on
PostgreSQL with the actual `mekuin_app` role.

### Phase 2O follow-up — approval scope immutability coverage

Expanded the manager-authorization privilege smoke test to assert that the runtime
role cannot rewrite the approval's business, branch, requester, approver, target,
action, expiry, or token digest, and cannot delete approval records. The only
permitted mutation remains setting `consumed_at` for atomic one-time consumption.
These assertions are authored but remain unexecuted until the PostgreSQL CI job runs.


### Phase 2N follow-up — approval token input hardening

- Approval token generation is centralized and uses 32 cryptographically random bytes encoded as 64 hexadecimal characters.
- Consumption rejects tokens that are not exactly 64 hexadecimal characters before querying PostgreSQL.
- Hexadecimal input is normalized to lowercase before digesting, matching the canonical issued token representation.
- Unit tests cover generated-token/digest agreement and malformed, non-hexadecimal and short token rejection.
- These tests are authored but not executed in this environment; run `go test ./...` with the pinned Go 1.24.13 toolchain.

### Phase 2P — HTTP catalog integration smoke coverage

Added `db/tests/api_catalog_smoke.sh` and wired it into the PostgreSQL CI job after
migrations, runtime-role provisioning and the three SQL smoke suites. The script
starts from a clean API registration, retains the session cookie, selects the
newly-created business/branch, creates and lists a category, verifies search
results, checks invalid pagination rejection, and reads the staff directory.
This exercises the HTTP router, cookie session, tenant selection, permission
middleware, tenant transaction, catalog SQL and response handling together.
The check is configured in CI but has not been executed in this local environment.
Manager PIN HTTP lifecycle tests (success, lockout, approval issuance/consumption)
remain a separate acceptance requirement.


### Phase 2Q follow-up — CI smoke-test correction and cross-tenant HTTP denial

A source review of the PostgreSQL authorization smoke test found that its tenant-A fixture creates three approval rows while the visibility assertion expected two. The assertion now expects three, matching the fixture, so the RLS test checks the intended tenant boundary rather than failing on its own fixture count. The HTTP smoke script now registers a second independent tenant and verifies that its authenticated owner receives HTTP 403 when attempting to select the first tenant's business and branch. Shell syntax and CI YAML parsing pass locally; neither the PostgreSQL nor HTTP test has been executed here.

### Phase 2R — staff membership provisioning and HTTP acceptance expansion

Added `POST /api/v1/staff/memberships` (`staff.manage`). A user must first register an account; an authorized business owner can then attach that existing account to the current business, assign one or more active branches, and create the membership in one tenant transaction. The handler rejects unknown accounts, duplicate memberships, malformed/duplicate branch IDs, and branches outside the selected business. It writes an audit event. This is an existing-account onboarding flow, not email invitation delivery or password setup; those remain deferred.

Added migration `011_staff_membership_lookup.sql`, which exposes only the UUID for an exact email match through a narrowly scoped `SECURITY DEFINER` function. It does not return account status, password hash, or profile information. Runtime role execution is granted; PUBLIC execution is revoked.

Expanded the HTTP CI smoke flow to cover staff membership creation, unknown-account rejection, cross-business branch rejection, duplicate membership rejection, profile creation, role/branch/PIN setup, successful manager PIN authorization, five failed PIN attempts, and lockout denial. The same smoke flow creates unit definitions, a directional conversion, products and a recipe, then asserts exact recipe cost/margin output. SQL smoke coverage continues to exercise one-time token consumption and replay rejection at the database layer; no sensitive sales or inventory handler exists yet to consume a token over HTTP.

OpenAPI is now version 0.4.0 with cookie authentication, structured error schema, request/response schemas for registration, tenant selection, staff membership/profile, manager authorization, units, conversions, products and recipes, plus the recipe-cost response. Remaining endpoint schemas should be expanded as each operational API family is implemented.

**Acceptance status:** all six original items are not yet fully accepted. Source and CI coverage have been expanded, but PostgreSQL 17 migrations/RLS/API smoke execution and Go 1.24.13 test/vet/build results still require a successful hosted CI run. The local environment cannot download Go 1.24.13 (network DNS unavailable) and has no PostgreSQL client or container runtime. Do not treat CI configuration or authored tests as successful execution evidence. Also outstanding: email invitation/acceptance flow; manager approval consumption by actual sensitive HTTP operations; broader permission/negative-path and transaction rollback coverage; full OpenAPI coverage for all route request/response bodies.

### Phase 2T — OpenAPI contract enforcement

The OpenAPI document now includes a shared structured error schema for every documented 4xx/5xx/default response. A Go contract test verifies the required Phase 2 paths and methods, tenant authentication declarations, request schema references and error response schema consistency. JSON validation passed during packaging. Go execution and PostgreSQL 17 runtime execution remain pending hosted CI; the local Go 1.24.13 toolchain download is blocked by DNS/network restrictions.


## Phase 2U — Staff invitation lifecycle

- Added `POST /api/v1/staff/invitations` for owner-authorized staff administrators. The API validates role and active branch assignments, stores only a SHA-256 token digest, and sends a 72-hour invitation email through configured SMTP.
- Added `POST /api/v1/auth/invitations/accept` for authenticated accounts. A narrowly scoped `SECURITY DEFINER` function checks token validity, expiry, account status and exact invited-email match, then creates the membership, branch assignments, audit event and acceptance timestamp atomically.
- Invitations are single-use. Expired, revoked, reused, email-mismatched and unavailable-branch invitations are rejected.
- SMTP configuration is required (`SMTP_HOST`, `SMTP_PORT`, `SMTP_FROM`; optional credentials). Delivery failure returns an error and the tenant transaction is rolled back. The response never exposes the raw invitation token.
- Important delivery caveat: SMTP submission is synchronous inside the request transaction. A rare database commit failure after successful SMTP submission can result in an email whose invitation was not committed. A transactional outbox is the recommended production hardening before high-volume delivery.
- Migration `012_staff_invitations.sql` adds tenant RLS, token constraints and the atomic acceptance function.
- The sales/refund/inventory approval integration remains intentionally unimplemented until the actual sale/payment/inventory domain and checkout transaction exist. The current token consumer remains a service primitive, not a standalone HTTP endpoint.


## Phase 2V — Complete staff invitation onboarding

- Added an invitation registration endpoint for recipients without an existing MEKU-In account. The invitation token determines the account email; the client cannot substitute another email. Possession of the emailed link verifies the email address.
- New invitees create an account without creating an unwanted owner business. The API creates the session, and the invitation page then accepts the same invitation. Existing account holders sign in and accept instead.
- Added the `/accept-invitation` Next.js page, sign-in return routing, registration-and-accept flow, and workspace continuation.
- Added invitation revocation for pending invitations. Revocation is tenant-scoped, permission-checked and audited.
- SMTP uses STARTTLS only, with connection/read-write deadlines. `SMTP_HOST`, `SMTP_PORT`, `SMTP_FROM` are required; credentials are optional when the relay permits unauthenticated delivery.
- Migration `012_staff_invitations.sql` adds `register_staff_invitee`, which creates an active account using the email embedded in the invitation. It does not create a business or membership; acceptance remains a separate atomic single-use function.
- CI now includes `db/tests/staff_invitation_rls_smoke.sql`, covering tenant invitation visibility, atomic acceptance, assigned branch access and replay rejection.
- Email is submitted synchronously to SMTP before the tenant transaction commits. A transactional outbox is still recommended to eliminate the rare case of successful email submission followed by database commit failure.
- Sales/refund/inventory approval consumption remains deferred until the corresponding business entities and checkout/stock transaction boundaries are implemented. Do not expose a standalone approval-consumption endpoint.


## Phase 2W — First transactional manager-approved operation

- Added the tenant-scoped `sales` record foundation and `POST /api/v1/sales/{saleID}/void`. This is deliberately a void operation on a sale created by a checkout/sales writer; this increment does not pretend to implement checkout, payment capture or inventory depletion.
- The void handler locks the sale row, verifies that it is completed and belongs to the selected branch, consumes the manager token bound to that exact sale ID/action/requester/business/branch, updates only void fields and writes the audit event in the same tenant transaction.
- Runtime database privileges permit updates only to `status`, `voided_by`, `voided_at` and `void_reason`; historical amount, identity, branch and creator fields remain immutable to the API role.
- CI HTTP smoke coverage now creates two fixture sales and verifies wrong-target rejection without consuming the token, successful void, replay rejection, five-failure PIN lockout and locked-PIN denial.
- Refunds and inventory-count approvals remain unintegrated because payment/refund and inventory ledger/count entities and transaction boundaries do not yet exist. They must follow the same consume-in-the-operation-transaction pattern when those domains are built.


## Phase 2V — Refunds, inventory reconciliation and transactional email outbox

### Manager-approved refunds

Added `POST /api/v1/sales/{saleID}/refunds`. The handler locks the sale row, requires a completed sale, calculates the already-recorded refund total while holding that lock, rejects amounts above the remaining sale total, consumes a target-bound `sales.refund` approval bound to the exact `target_amount_minor`, writes the refund record and audit event in the same tenant transaction. The approval schema and migration 015 add amount binding; runtime privileges permit reading/inserting that field but still permit updates only to `consumed_at`. The response explicitly reports `recorded_pending_settlement`: this records the approved refund obligation but does not claim that an external M-Pesa/card/cash payout has completed. Refunds remain immutable records; repeated approval tokens cannot be replayed.

### Inventory count and reconciliation

Added draft inventory counts with product-level expected ledger quantity snapshots and exact decimal counted quantities. One draft count per branch is allowed to prevent overlapping reconciliation sessions. Reconciliation locks the count and its lines, consumes a target-bound `inventory.count.approve` token, writes only non-zero compensating `count_reconciliation` movements, marks the count reconciled and appends an audit event in the same transaction. The stock quantity is the sum of immutable movement deltas; the count process does not overwrite historical movements or product cost fields. Negative counted quantities, malformed decimal values, duplicate products and inactive/cross-tenant products are rejected.

### Transactional email outbox

Invitation creation now inserts the invitation and email payload into `email_outbox` in the same tenant transaction. A background worker claims committed messages with `SKIP LOCKED`, submits them over SMTP with STARTTLS, retries failures with exponential delay, and marks exhausted messages failed. Successful and terminally failed messages have their body cleared. The runtime role cannot directly read or update outbox rows; narrow `SECURITY DEFINER` claim/complete functions provide worker access. The queue is at-least-once: a process crash after SMTP acceptance but before marking a message sent can cause a duplicate email. SMTP delivery is asynchronous and requires the existing SMTP environment variables.

### Acceptance automation added

- `db/tests/refunds_inventory_outbox_rls_smoke.sql` covers runtime-role RLS and writes for refund, inventory and outbox records.
- The API CI smoke script covers outbox enqueue, partial refunds, refund replay, refund ceiling, count creation, manager-approved reconciliation, replay/state rejection and the resulting movement balance.
- OpenAPI v0.8.0 now documents refund and inventory-count operations and their request/response schemas.
- CI includes PostgreSQL 17 migration/RLS/API integration, Go 1.24.13 test/vet/build, and a separate Node 22 Next.js lint/production-build job.

### Acceptance status and limitations

Implementation and CI coverage are authored. They have **not** been executed in this local environment: it has Go 1.23.2, no cached dependency checksums, no PostgreSQL client/container runtime, and no installed Next.js dependencies; outbound DNS/network is unavailable. Therefore PostgreSQL 17 migrations/RLS/HTTP tests, Go 1.24.13 tests/vet/build and Next.js lint/build remain pending actual hosted CI results. The refund endpoint records the refund obligation only; external payout settlement and payment-provider reconciliation are not implemented here. Inventory movements currently cover count reconciliation; purchase receiving, sale consumption and wastage movement integrations remain later operational increments. Email delivery is at-least-once and needs monitoring/dead-letter operations before high-volume production use.

## Phase 2Z — Acceptance hardening and inventory reconciliation permissions

- Split the permission to submit a count reconciliation (`inventory.count.reconcile`) from the permission to approve it (`inventory.count.approve`). Stock controllers and managers may submit the reconciliation request, but the handler still requires a separate, single-use manager approval token bound to that exact count. The approval itself remains consumed inside the reconciliation transaction.
- Added role-permission unit cases for manager and stock-controller reconciliation access. The stock-controller role still cannot issue manager approvals.
- CI now asserts the PostgreSQL server is major version 17, reports Goose migration status, verifies all 15 migrations are applied, and runs race-enabled Go tests in addition to normal tests, vet and production build.
- API integration logs are uploaded as a GitHub Actions artifact when the PostgreSQL/API job fails, and the API server log is printed on failure to improve diagnosis.
- Local checks performed for this increment: `gofmt`, shell syntax, YAML parsing, OpenAPI JSON parsing and ZIP integrity. Go 1.24.13 tests/race/vet/build, PostgreSQL 17 migrations/RLS/API smoke and Next.js lint/build still require the hosted workflow; this environment has Go 1.23.2, no PostgreSQL client/container runtime and no installed frontend dependencies.

## Phase 2 closure and Phase 3 Selling kickoff

Phase 2 implementation coverage is carried forward in the v40 source archive. Its external runtime acceptance remains dependent on the PostgreSQL 17, Go 1.24.13 and Next.js CI results; a local command run is not represented here as a hosted CI result.

### Phase 3A — server-priced POS basket foundation

- Added `POST /api/v1/sales/quote`, protected by `pos.use`.
- The API loads current active/sellable catalogue prices; client-provided prices and totals are ignored because they are not accepted in the request schema.
- Uses integer minor-unit arithmetic with explicit overflow checks and rejects zero, negative, fractional-by-schema, duplicate and excessive quantity lines.
- Returns business currency, immutable quote line snapshots and a common total structure.
- Tax, service charge and discounts remain unapplied pending explicit approval of the pricing policy. This is intentional; example rates in the specification are not silently adopted.
- Added pure Go unit tests for line multiplication, basket aggregation, invalid quantities and overflow.
- Extended the authenticated HTTP smoke test to quote a two-unit basket and assert exact totals.
- Published the route and request/response schemas in OpenAPI 0.9.0.

Phase 3A is a quote/calculation foundation, not a completed checkout. Payment capture, persisted orders/sales, idempotent checkout and recipe-driven stock deduction are the next selling increments. Before stock deduction is enabled, negative-stock policy, unit/portion rounding, stock valuation and payment verification semantics must be resolved explicitly.
