# MEKU-In build plan

## Phase 1 — Foundation
Monorepo, Next.js app, Go/Chi API, PostgreSQL, Docker, migrations, health checks, auth/session foundation, business/branch/membership model, tenant middleware/RLS, permissions, workspace shell and OpenAPI contract.

Acceptance:
- registration and business/branch setup
- owner membership creation
- authenticated workspace loading
- suspended account rejection
- tenant isolation through API and database policies
- clean database bootstrap and local run

## Phase 2 — Catalog and configuration
Branches, staff, role/branch assignment, manager PIN, products/categories/units/liquid conversions, suppliers, recipes/costing/margin and tables.

## Phase 3 — Selling and payments
POS, order lifecycle, central money engine, checkout transaction, discounts/manager approval, split payments, payment references/verification, receipts, voids/refunds, stock deductions, audit and idempotency.

## Phase 4 — Inventory and procurement
Stock ledger/balances, wastage/breakage/adjustments/transfers, suppliers, purchase orders/partial receiving, stock counts/blind counts/approval and reconciliation.

## Phase 5 — Cash, reporting and control
Shifts/cash-up, expenses, reports, alerts, append-only audit views, settings, integrations configuration and print layouts.

## Phase 6 — Offline POS
IndexedDB, durable outbox, client references, idempotent replay, sync states, conflict/recovery. Offline-authorised actions must be explicitly defined.

## Phase 7 — Platform and production
Super-admin, invitations, account lifecycle, demo data, integrations, security/performance, monitoring, backup/restore, deployment and support docs.

### Phase 7A — Read-only platform administration foundation
- [x] Server-side super-admin authorization revalidated against the database on every request.
- [x] Read-only, paginated business directory with active/suspended filtering and minimal aggregate metadata.
- [x] No public super-admin promotion endpoint; bootstrap remains a controlled database-owner operation.
- [ ] Execute Go tests with pinned Go 1.24.13 and API authorization tests.
- [ ] Add a dedicated platform UI after platform support workflows and audit requirements are approved.
- [ ] Account lifecycle mutations, support access, demo tenant provisioning and retention/deletion remain later increments.

## Explicitly unresolved business decisions
Do not silently hardcode:
- VAT inclusive/exclusive treatment and service-charge tax treatment
- discount ordering and rounding policy
- stock valuation method
- negative-stock policy
- refund stock-restoration and approval policy
- offline action/approval boundaries
- payment verification lifecycle
- report metric definitions
- super-admin deletion/retention policy

### Phase 2A schema deliverable

- [x] Tenant-safe schema for staff profiles, role permissions, UOM/conversions, categories, products, suppliers, recipes/ingredients and branch tables.
- [x] Exact integer minor-unit product prices and exact NUMERIC quantities.
- [x] Forced tenant RLS and composite tenant-safe foreign keys for catalog tables.
- [x] SQL smoke-test script for core constraints and tenant boundaries.
- [ ] Execute migration and smoke test against PostgreSQL 17.
- [ ] Add Go catalog services/handlers and API tests (Phase 2B).
- [ ] Add catalog management screens (Phase 2C).

### Phase 2B backend increment (v15)

Implemented tenant-protected catalog endpoints under `/api/v1/catalog`:

- `GET/POST /categories`
- `GET/POST /units`
- `GET/POST /products`
- `GET/POST /suppliers`
- `GET/POST /tables` (scoped to the selected branch)
- `GET/POST /recipes` (recipe header and ingredient rows created in the same request transaction)

All handlers use the transaction and tenant context established by `requireTenant`; business and branch identifiers are taken from that authenticated context, not request fields. Writes use parameterized SQL, bounded JSON decoding, database constraints, and stable API error codes. Product prices are integer minor units; quantities remain decimal strings and PostgreSQL NUMERIC. The database remains the final validator for cross-tenant references, unit dimensions, and liquid serving constraints.

The tenant middleware now buffers responses and commits only successful (2xx/3xx) handler responses. Error responses roll back the request transaction, and a failed commit is returned as 503 before any success body is sent. This also makes multi-row recipe creation atomic.

This is a first backend slice, not full Phase 2B completion. Update/deactivate endpoints, staff profile and role assignment APIs, conversion CRUD, recipe costing/margin calculation, API-level integration tests against PostgreSQL, and OpenAPI expansion remain outstanding.


## Phase 3B — Transactional cash checkout

Implemented server-priced cash checkout, immutable sale-line and payment snapshots, branch-scoped idempotency and HTTP replay coverage. Recipe stock deduction, provider payment verification, taxes/service charge/discounts and order/table workflows remain explicit follow-up work. See `PHASE_3B_CHECKOUT.md`.

## Phase 3C — Recipe-backed stock consumption

Checkout now consumes recipe ingredients through immutable `sale_consumption` inventory movements in the same tenant transaction as sale lines and payment records. Ingredient quantities are converted directionally from recipe units to product base units using the configured conversion factor; missing conversions, quantities that cannot be represented exactly at eight decimal places, and insufficient branch stock reject checkout. Ingredient balance locks serialize concurrent recipe sales. Products without an active recipe are not automatically stock-tracked in this increment because the product stock-tracking/negative-stock policy is not yet approved. This is not a complete inventory/POS release; see `PHASE_3C_RECIPE_STOCK.md`.


## Phase 3D — POS order lifecycle (v44)
- Persist branch-scoped walk-in, table and tab order drafts with server-priced line snapshots.
- List open/held orders and retrieve individual order details.
- Hold open orders without creating sales, recording payments, recognizing revenue or consuming stock.
- Order checkout/settlement, line editing, reopening and cancellation remain subsequent work.


## Phase 3E — POS order editing and lifecycle (v45)

Implemented server-priced line replacement for open orders, held-order reopening, safe cancellation, audited transitions, and a database constraint preventing simultaneous active orders on one table. Direct order checkout, table transfer, split tender and post-checkout table release remain outstanding.


## Phase 3F — Order-to-sale checkout (v46)
- Added `POST /api/v1/sales/orders/{orderID}/checkout` with mandatory `Idempotency-Key`.
- The endpoint delegates to the existing transactional checkout implementation; order lines are loaded from the locked order and client-supplied line prices/quantities are not accepted.
- Sale snapshots, recipe ingredient movements, cash payment, checkout idempotency, audit records, and order `checked_out`/`checked_out_sale_id` linkage commit in the same tenant transaction.
- Only open orders can be checked out; duplicate requests replay the original sale through the shared idempotency record.
- Table occupancy is released by the existing active-order partial unique index when the order moves to `checked_out`.
- Split tenders and non-cash provider verification remain disabled; checkout continues to accept cash only.


### Phase 3G — POS order operations UI (v93)

- [x] Add a branch-scoped active order register for open and held orders.
- [x] Add order detail inspection with server-priced line editing (product IDs and quantities only).
- [x] Add hold, reopen and cancellation actions using existing audited API transitions.
- [x] Add cash checkout through the existing idempotent transactional checkout endpoint.
- [x] Expose Orders from the authenticated workspace for POS users.
- [x] Keep checkout confirmation explicit and require the server-side active cash shift / stock checks.
- [ ] Add authenticated browser acceptance for order lifecycle and cash checkout; external PostgreSQL, Go 1.24.13 and Next.js production checks remain release gates.
- [ ] Payments ledger/verification UI, receipt rendering, void/refund UI, discount authorisation UX and broader split-tender support remain separate Phase 3 increments.

### Phase 3H — M-Pesa payment operations UI (v94)

- [x] Add branch-scoped, paginated M-Pesa attempt register with status filtering.
- [x] Display masked phone numbers and omit callback payloads/secrets.
- [x] Expose provider reconciliation only when the server marks an attempt eligible.
- [x] Expose atomic settlement only after provider confirmation and server-side eligibility checks.
- [x] Add investigation notes for review-required attempts; notes do not alter payment or sale state.
- [ ] Daraja sandbox/production credentials, callback URL verification, real-provider acceptance and operational runbook sign-off remain external gates.
- [ ] General cash/card payment ledger, receipt printing, void/refund UI and sales history remain separate increments.

### Phase 5A backend and web increment

- [x] Branch/user cash shifts with a single-open-shift database constraint.
- [x] Attribute cash tenders to the active shift; reject cash checkout without an open shift.
- [x] Record immutable reasoned drawer paid-in/out movements.
- [x] Calculate expected drawer cash and close variance transactionally; append audit events.
- [x] Expose a printable shift summary page.
- [ ] PostgreSQL 17 migration/RLS execution and full Go/Next.js CI verification.
- [ ] Refund cash tender attribution, alerts and audit-ledger browsing remain later Phase 5 increments.


### Phase 5B backend and web increment

- [x] Branch-scoped expense categories and expense register.
- [x] Separate approval/rejection lifecycle with reasoned, append-only review events and creator self-review prevention.
- [x] Approved expense payment recording with immutable payment history.
- [x] Atomic cash expense payment + shift paid-out movement + audit record; non-cash methods remain outside drawer cash.
- [x] Branch expense summary grouped by category, currency and status, with date filters.
- [ ] PostgreSQL 17 migration/RLS execution and full Go/Next.js CI verification.
- [ ] Partial payments, attachments, configurable approval thresholds, budgets and audit-ledger UI remain later increments.


### Phase 5C backend and web increment

- [x] Durable branch-scoped stock-out, low-stock, expense approval, supplier invoice review and cash variance alerts.
- [x] Alert acknowledgement/resolution workflow with append-only alert events and audit records.
- [x] Business-wide append-only audit ledger API with action/entity/search filters.
- [x] Workspace navigation and alert/audit browsing pages.
- [x] Forced tenant RLS, active-alert deduplication and least-privilege alert event grants.
- [ ] PostgreSQL 17 migration/RLS smoke execution, Go 1.24.13 test/build and Next.js production build.
- [ ] Background alert scheduling, push/email delivery, configurable severity thresholds and branch-attributed historical audit browsing remain future increments.

### Phase 5D backend and web increment

- [x] Branch-scoped alert enable/disable settings with tenant RLS.
- [x] Configurable cash variance critical threshold using integer minor units.
- [x] Manager/owner settings permissions, audit event on update and settings UI.
- [ ] PostgreSQL 17 migration/RLS smoke execution, Go 1.24.13 test/build and Next.js production build.
- [ ] Receipt layout customization, provider credential management and background alert delivery remain separate future increments.

### Phase 5E operational reporting and print increment

- [x] Date-filtered closed cash-shift history, scoped to own shifts or branch by `shifts.manage`.
- [x] Operational expense totals grouped by currency and workflow status, without currency conversion.
- [x] `/reports` page with cash reconciliation detail, date filters and browser print stylesheet.
- [x] OpenAPI reporting contracts and workspace navigation.
- [ ] PostgreSQL 17 execution, Go 1.24.13 tests/build, Next.js production build and authenticated staging acceptance.
- [ ] CSV export, scheduled reports, advanced sales/profit metrics and statutory accounting statements remain future work.

### Phase 5F report exports and acceptance increment

- [x] Add CSV export for currently loaded cash-shift history rows.
- [x] Add CSV export for expense summary totals grouped by currency and status.
- [x] Quote CSV cells, escape embedded quotes, and neutralize common spreadsheet formula prefixes.
- [x] Document export scope/limits and consolidated Phase 5 acceptance checklist.
- [ ] Complete PostgreSQL 17, Go 1.24.13, Next.js production and authenticated staging release gates before Phase 6.

Phase 5 source feature work is feature-complete for the current planned scope. It is not production-accepted until the external release gates in `PHASE_5F_REPORT_EXPORTS_ACCEPTANCE.md` pass.

### Phase 5 verification and hardening pass (v67)

- [x] Harden CSV formula-prefix detection against leading ASCII whitespace/control characters before formula operators.
- [x] Clean-archive static verification: Go formatting, Phase 5 migration inventory, TSX syntax transpilation, CI YAML parsing, and ZIP integrity.
- [ ] Go 1.24.13 tests/race/vet/build remain blocked in the current environment because the pinned toolchain could not be downloaded.
- [ ] PostgreSQL 17 migration and RLS smoke execution remain blocked because PostgreSQL client/server tooling is unavailable.
- [ ] Next.js lint/build remain blocked because npm dependency installation did not complete; no lockfile is present in the archive.
- [ ] Authenticated staging acceptance and data reconciliation remain outstanding.

See `PHASE_5_VERIFICATION_HARDENING.md`. Phase 5 remains not production-accepted until all release gates pass.

### Phase 6A — Offline POS order-draft foundation

- [x] IndexedDB product cache and branch/business-scoped durable outbox.
- [x] POS draft capture and connectivity/sync state UI.
- [x] Automatic replay with stable client references and server-side idempotency.
- [x] Payload fingerprint mismatch rejection and per-reference transaction lock.
- [x] Explicitly keep payments, completed sales, stock deduction and approvals online-only.
- [ ] Durable basket restoration, app-shell service worker, table-cache strategy and browser recovery acceptance tests.
- [ ] Agree offline cash/payment, shift attribution, stale-price and approval policy before expanding offline-authorised actions.

See `PHASE_6_OFFLINE_POS_FOUNDATION.md`.

### Phase 6B — Offline outbox recovery and correction

- [x] Correct definitively blocked drafts by restoring their contents to the POS basket and issuing a fresh client reference on requeue.
- [x] Discard blocked drafts and clear locally retained synced entries without changing server-side orders.
- [x] Validate local quantities and duplicate product lines before queueing.
- [x] Add an in-page sync mutex to prevent overlapping queue replay loops.
- [ ] Table selection/cache, cross-tab coordination, storage quota/retention controls and browser recovery automation.
- [ ] Decide offline payment and shift attribution policy before allowing completed offline sales.

See `PHASE_6B_OFFLINE_OUTBOX_RECOVERY.md`.

### Phase 6C — Offline POS scope safety, tables and recovery safeguards

- [x] Verify authenticated business/branch scope before replay; block mismatched drafts without posting them.
- [x] Classify replay failures using HTTP status instead of parsing error text.
- [x] Use Web Locks for cross-tab replay coordination where supported, retaining the per-tab mutex.
- [x] Cache branch-scoped tables in IndexedDB v2 and support table order drafts with server-side revalidation.
- [x] Expose read-only table listing to POS users while keeping table administration permission-protected.
- [x] Handle storage quota errors without automatically deleting unsynced work; manually clear synced records.
- [ ] Browser acceptance and external Go/PostgreSQL/Next.js/staging checks.

See `PHASE_6C_OFFLINE_POS_SCOPE_AND_TABLES.md`.


### Phase 6D — Offline POS recovery and acceptance hardening

- [x] Recover interrupted `syncing` drafts to `pending` after reload while preserving the idempotency reference.
- [x] Guard against saving basket lines absent from the loaded active/sellable product cache.
- [x] Label cached basket totals as indicative and clarify server-side revalidation.
- [x] Add a browser acceptance matrix covering offline reload, timeout-after-commit, scope switching, stale products/tables, concurrent tabs, quota and permissions.
- [ ] Complete browser acceptance and external Go/PostgreSQL/Next.js/staging checks.

See `PHASE_6D_OFFLINE_POS_RECOVERY_ACCEPTANCE.md`.


### Phase 6E — Offline POS operator recovery and draft inspection

- [x] Add expandable outbox draft inspection with business/branch scope, table, server reference, attempt count and timestamps.
- [x] Show cached line descriptions and indicative values without guessing missing product data or prices.
- [x] Make local discard/clear actions surface IndexedDB failures.
- [x] Preserve blocked-draft correction through a fresh client reference; do not silently retry blocked payloads.
- [ ] Complete browser acceptance and external Go/PostgreSQL/Next.js/staging checks.

See `PHASE_6E_OFFLINE_POS_OPERATOR_RECOVERY.md`.

## Phase 6F — Offline POS basket and catalogue safeguards

Implemented an operator-triggered catalogue refresh, safe integer price/quantity checks, table selection validation against the branch cache, and stricter successful-sync response validation. Offline POS remains limited to order drafts; server-side validation and the existing idempotency contract remain authoritative. See `docs/PHASE_6F_OFFLINE_POS_BASKET_AND_CATALOGUE_GUARDS.md`.

## Phase 6G — Offline POS concurrent recovery safeguards

- [x] Avoid resetting recent `syncing` records during startup recovery; recover only records at least two minutes stale (or with invalid timestamps).
- [x] Add an operator-triggered **Recover stale** action with visible result feedback.
- [x] Preserve the original idempotency reference during stale recovery.
- [ ] Verify concurrent tabs, slow requests, timeout-after-commit and stale recovery in a real browser.
- [ ] Complete external Go/PostgreSQL/Next.js/staging checks.

See `docs/PHASE_6G_OFFLINE_POS_CONCURRENT_RECOVERY.md`.

### Phase 6H — Offline POS server-outcome reconciliation
- Expose the client idempotency reference in the branch-scoped POS order list response.
- Before replaying pending/interrupted drafts, reconcile them against visible server orders by exact client reference and mark confirmed matches as synced.
- Preserve the original reference for any unmatched replay; the create endpoint's idempotency guard remains the final duplicate-prevention mechanism.
- Limitation: the existing order list is limited to the latest 100 open/held orders. A missing match is not proof that the server did not commit; retry remains safe because the create endpoint performs idempotent lookup.
- Payments, checkout, stock movement and completed sales remain outside offline scope.

### Phase 6I — Exact offline order-reference reconciliation

- Added `POST /api/v1/sales/orders/reconcile`, requiring `pos.use` and the active authenticated tenant/branch context.
- Accepts 1–100 unique UUID client references and returns exact matches across all POS order statuses, not only the latest 100 open/held orders.
- Lookup is constrained by both `business_id` and `branch_id`; it does not expose cross-tenant or cross-branch references.
- Offline outbox replay now performs one batch lookup for queued references before attempting creates. A match records the existing server order ID and prevents a duplicate create request.
- A missing match remains inconclusive only at lookup time; the existing create endpoint's stable client-reference idempotency remains the final protection against concurrent/late commits.
- No schema migration is required. Payments, checkout, stock deduction and completed-sale behaviour are unchanged.

### Phase 6J — Offline POS sync diagnostics and history

- [x] Add backward-compatible, bounded local sync history to outbox drafts.
- [x] Record meaningful draft state/attempt/error/server-confirmation transitions.
- [x] Display recent sync history in expanded draft inspection.
- [x] Keep diagnostics local-only and avoid changing server, payment, stock or completed-sale behaviour.
- [ ] Verify legacy IndexedDB records, failure/recovery timelines, 30-event retention and multi-tab behaviour in a real browser.
- [ ] Complete external Go/PostgreSQL/Next.js/staging checks.

See `docs/PHASE_6J_OFFLINE_POS_SYNC_DIAGNOSTICS.md`.

### Phase 6K — Atomic offline POS diagnostics
- [x] Make outbox diagnostic read/modify/write atomic within one IndexedDB transaction to prevent concurrent-tab history lost updates.
- [x] Preserve the 30-event bound and legacy records without history.
- [x] Surface IndexedDB transaction failures to callers; do not treat failed local writes as successful.
- [ ] Verify concurrent-tab transition races, transaction aborts, quota failures and legacy records in real browsers.
- [ ] Complete external Go/PostgreSQL/Next.js/staging checks.

See `PHASE_6K_OFFLINE_POS_ATOMIC_DIAGNOSTICS.md`.

### Phase 6L — Offline POS atomic draft claims

Added an atomic IndexedDB claim before replaying each draft. Recent `syncing` rows are skipped, stale rows can be reclaimed, and the attempt counter plus diagnostic event are committed with the claim. This complements Web Locks and server idempotency; it is not a cross-device distributed lock. See `docs/PHASE_6L_OFFLINE_POS_ATOMIC_CLAIMS.md`.

### Phase 6M — Offline POS automated policy tests
- [x] Extract claim/stale-recovery and bounded-history policies into a shared pure module used by production code.
- [x] Add Node built-in automated tests for claim eligibility, stale thresholds, malformed/future timestamps and history retention.
- [x] Add `npm run test:offline`.
- [ ] Add real IndexedDB/browser tests for atomic transactions, concurrent tabs, interrupted network requests and service-worker reloads.
- [ ] Complete external Go/PostgreSQL/Next.js/staging checks.

See `docs/PHASE_6M_OFFLINE_POS_AUTOMATED_POLICY_TESTS.md`.

### Phase 6N — Offline POS IndexedDB integration tests

Added an IndexedDB-backed integration suite using `fake-indexeddb` for draft persistence, branch scoping, atomic concurrent claims, state/history updates, stale recovery, cache round trips and targeted deletion. Run with `npm install` followed by `npm run test:offline` from `apps/web`. Browser-engine, quota, service-worker and live server acceptance remain separate checks.

### Phase 6O — Offline POS service-worker acceptance

- [x] Add deterministic service-worker tests for install/activation, API/auth/tenant bypass, non-GET and cross-origin bypass, static asset caching, and offline navigation fallback.
- [x] Include service-worker tests in `npm run test:offline`.
- [x] Document real-browser acceptance scenarios and explicitly separate mocked tests from browser verification.
- [ ] Run Chromium/Firefox/WebKit offline, reload, storage and multi-tab acceptance.
- [ ] Complete external Go/PostgreSQL/Next.js production and authenticated staging gates.


### Phase 6P — Browser storage pressure and persistence safeguards

Added browser storage usage/quota visibility and an explicit operator-triggered persistent-storage request. Browser estimates and persistence grants are advisory; they do not guarantee retention. IndexedDB quota/transaction failures remain visible to the operator, and the application never auto-deletes pending, syncing, or blocked drafts to reclaim space. Operators may manually clear synced history. See `docs/PHASE_6P_OFFLINE_STORAGE_SAFEGUARDS.md`.

### Phase 6Q — Offline action boundary and operational recovery

Confirmed the supported offline workflow remains local order-draft capture only. The client never treats a draft as a sale, payment, stock movement, shift movement, refund, discount approval, or manager-authorised action. Cached prices and tables remain advisory; server validation and tenant context are authoritative on replay. Storage eviction, user-cleared site data, browser profile loss, and device loss can still remove local drafts; this limitation is explicit in the POS operator interface and release checklist.

### Phase 6R — Phase 6 implementation closeout and release gates

The technical Phase 6 source scope is implemented through 6Q: scoped IndexedDB outbox and caches, stable-reference idempotent replay, tenant-scope checks, reconciliation, atomic draft claims/history, service-worker cache boundaries, operator recovery, automated policy/service-worker tests, and storage safeguards. This is implementation closeout, not production acceptance. External gates remain: install web dependencies and run the complete offline test command, run full TypeScript/Next.js checks, execute browser-engine tests on Chromium/Firefox/WebKit where supported, verify Go/PostgreSQL migrations/RLS and authenticated staging, and complete the manual airplane-mode/lost-response/two-tab/tenant-switch/storage-eviction scenarios.

Business decisions deliberately deferred: whether any offline tender is permitted; shift attribution and reconciliation for offline cash; offline discount/approval authority; policy for price changes, deactivated products, unavailable stock and table occupancy; and recovery expectations after browser data is cleared. Until approved, offline payments and completed sales remain disabled. See `docs/PHASE_6R_CLOSEOUT_AND_RELEASE_GATES.md`.

## Phase 7 — Platform administration

Phase 7A–7D source implementation is consolidated in `PHASE_7_PLATFORM_ADMIN_CLOSEOUT.md`: read-only business/user directories, reason-required reversible business/user status controls, session revocation on user suspension, active-owner protection, and append-only platform audit events. Production acceptance remains gated by PostgreSQL privilege/RLS tests, Go 1.24.13 checks, Next.js checks and authenticated staging verification. Billing, ownership transfer, impersonation and destructive deletion are deliberately excluded.

### Phase 7E — Platform admin query hardening
- Bounded pagination and status filtering for the platform user directory.
- Bounded pagination and entity filtering for platform audit history.
- Explicit runtime-role revocation of audit-table mutation privileges in migration 042.
- Unit coverage for pagination parsing boundaries.
- Platform administration remains API-first; no billing, impersonation, ownership transfer, destructive deletion, or UI console is introduced.

### Phase 8A — Platform administration console
- [x] Add `/platform` console for business/user directories and platform audit history.
- [x] Add bounded pagination and status/entity filters using existing platform APIs.
- [x] Add reason-required business/user suspend/reactivate confirmation flows.
- [x] Preserve API-side super-admin authorization; no client-side role is treated as authority.
- [ ] Complete full Next.js build, Go 1.24.13 suite, PostgreSQL privilege/migration checks and authenticated staging acceptance.
- [ ] Billing, subscriptions, support impersonation, ownership transfer and deletion remain separate explicitly approved scopes.

See `docs/PHASE_8A_PLATFORM_ADMIN_CONSOLE.md`.

## Phase 8 — Platform administration and onboarding

- [x] 8A Platform administration console for business/user directories and audit history.
- [x] 8B Transactional business workspace provisioning for an existing active owner account.
- [x] 8C Console provisioning workflow with mandatory reason and post-create refresh.
- [x] 8D Security boundaries and deferred onboarding/billing capabilities documented.
- [ ] External acceptance: restricted-role PostgreSQL/RLS provisioning test, Go test suite, Next.js production build, and authenticated browser test.

See `docs/PHASE_8_PLATFORM_ONBOARDING_CLOSEOUT.md` for API contract and explicit exclusions.

### Phase 2C — Product catalogue screen increment (v89)
- [x] Add permission-aware `/products` route with tenant-scoped list, search and unit/liquid filters.
- [x] Add product creation using existing category/unit/product APIs; preserve integer minor-unit money and exact decimal quantities.
- [x] Add soft deactivation without deleting historical product references.
- [ ] Add category/unit/conversion administration screens and product edit workflow.
- [ ] Verify through PostgreSQL-backed authenticated staging and full Next.js build.

See `PHASE_2C_PRODUCT_CATALOGUE_UI.md`.

### Phase 2D — Catalogue configuration UI (v90)
- [x] Add tenant-authenticated category administration with create and safe deactivation.
- [x] Add unit-of-measure administration for count, mass and volume dimensions, with safe deactivation.
- [x] Add directional unit-conversion creation and read-only listing; reverse factors are not inferred.
- [x] Link configuration from the product catalogue.
- [ ] Add product edit workflow after full-state loading and immutable-field rules are exercised against staging.
- [ ] Add recipe builder/costing UI, supplier directory, table configuration, and staff/role/PIN management UI.
- [ ] Run authenticated PostgreSQL-backed acceptance and full Next.js production build.

See `PHASE_2D_CATALOG_CONFIGURATION_UI.md`.

## Phase 2 completion pass — catalogue operations UI
- Phase 2C: product catalogue UI (`/products`).
- Phase 2D: category, unit and directional conversion configuration (`/catalog/settings`).
- Phase 2E: recipe creation and live costing (`/recipes`), supplier maintenance (`/suppliers`), branch table configuration (`/tables`), and staff invitations/role/profile UI (`/staff`).
- Recipe cost is a live estimate from current product catalogue cost prices, not stock valuation or historical cost. Directional conversions are required and not inferred.
- Remaining Phase 2 acceptance is external: PostgreSQL migration/RLS tests, role-by-role API/UI checks, invitation email delivery/acceptance, branch-scope checks, and full web build in an installed dependency environment.

### Phase 2 — Catalog and configuration UI completion pass (v91)

- [x] Product catalogue UI and category/unit/directional-conversion configuration UI (v89–v90).
- [x] Recipe builder UI: sellable output, output quantity, multiple ingredient products, explicit ingredient units/quantities and preparation notes.
- [x] Recipe cost/margin detail uses the existing server calculation. The UI explicitly labels it as a live estimate from current `products.cost_price_minor`, not weighted-average inventory valuation or historical sale cost.
- [x] Supplier directory UI with create, edit and soft-deactivate; balances are not fabricated and remain derived from purchasing/payables records.
- [x] Branch-scoped table configuration UI with create, edit and soft-deactivate. Occupancy/tab state remains owned by the POS order lifecycle.
- [x] Staff and role UI already present in `/staff`; invitation, profile and role actions use the existing permission-guarded API. Manager PIN hashes are never read back or exposed.
- [x] Added workspace links to recipes, suppliers, tables and owner-only staff administration.
- [ ] Branch creation/maintenance UI and multi-branch staff assignment editor are not yet exposed as a complete administration workflow; workspace branch selection and invitation assignment remain available through existing APIs.
- [x] Added a one-time manager PIN setting form to `/staff`; the PIN is sent over the authenticated API, bcrypt-hashed server-side and never returned to the browser.
- [ ] PostgreSQL 17 migration/RLS execution, Go 1.24.13 tests/build, Next.js production build and authenticated multi-tenant staging acceptance remain external release gates.

Phase 2 is feature-complete for the currently exposed catalog, recipe, supplier, table and staff workflows, but it is not release-accepted. The explicitly listed branch administration, manager PIN UI and external verification items remain open and must not be represented as completed.

### Phase 2 — Completion pass (v92)

- [x] Branch directory endpoint: tenant-scoped list, create and update/activate/deactivate. Branch rows are soft-deactivated; no destructive delete path exists.
- [x] Branch administration screen at `/branches`, linked from the owner workspace.
- [x] Staff directory now supports multi-branch assignment using the existing atomic membership-branch replacement endpoint; inactive branches cannot be newly assigned.
- [x] Staff invitation supports multiple selected active branches; role changes and manager PIN setup remain server-authorized.
- [x] Recipe cost endpoint exposes exact decimal minor-unit estimates, gross margin and percentage, explicitly labelled as current catalogue-cost estimates rather than inventory valuation.
- [x] Phase 2 UI coverage: products, categories, units/conversions, recipes/costing, suppliers, tables, staff and branches.
- [x] OpenAPI contract and route coverage updated for branch endpoints.
- [ ] Run pinned Go 1.24.13 tests, migrations and tenant/RLS smoke tests against PostgreSQL 17.
- [ ] Install frontend dependencies and run the full Next.js production build/type check.
- [ ] Authenticated staging acceptance: create a second branch, assign staff to both branches, verify branch-scoped catalogue/table visibility, and confirm a cross-business branch ID is rejected.

Phase 2 implementation is complete in source as of v92. External runtime/database/staging acceptance remains outstanding and is not represented as passed.
