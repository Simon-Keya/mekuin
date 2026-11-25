# Build specification completion and acceptance matrix

This document records the implementation increment after the inventory/stock-control work. The Developer Build Specification and Screen Booklet remain the scope authority.

## Implemented in this increment

### Weighted-average inventory costing

- Migration `046_inventory_weighted_average_valuation.sql` adds a per-business, per-branch, per-product valuation balance and an append-only valuation movement ledger.
- An `AFTER INSERT` trigger on the immutable inventory movement ledger updates quantity, moving weighted-average unit cost and stock value transactionally.
- Purchase receipt movements carry their receipt unit cost. Transfers carry the source branch's moving average into the destination branch. Consumption and ordinary losses reduce value at the current average. Void/refund reversals restore the original sale-consumption valuation cost where the source movement is available.
- Existing movements are replayed in chronological order during migration to establish opening valuation history; receipt unit costs are recovered from receipt lines.
- The runtime role can read valuation records but cannot directly mutate the valuation tables.

### Stock valuation reporting

- `GET /api/v1/inventory/valuation` returns branch stock value at cost, low/out-of-stock counts and month-to-date wastage value.
- Inventory balances include average unit cost and stock value per product. The Inventory screen shows the KPI figures and per-product valuation.
- `GET /api/v1/reports/summary?tab=stock-value` returns product-level valuation rows and total stock value.

### Stock-taking workflow

- Migration `047_inventory_count_submission.sql` adds `submitted` state, submission actor/time and one-open-count-per-branch protection.
- Draft counts can be edited with `PATCH /api/v1/inventory/counts/{countID}`.
- Draft counts must be explicitly submitted through `POST /api/v1/inventory/counts/{countID}/submit` before reconciliation.
- Manager approval remains separately PIN-authorized and is required to post variance movements.
- Blind counts continue hiding expected quantity and variance until the count is reconciled.

### Historical reconciliation

- Migration `048_historical_recipe_consumption_snapshots.sql` adds immutable checkout-time snapshots of sold product, recipe output quantity, normalized ingredient quantity and actual consumed quantity. Direct-sale products with branch stock history now also create a direct-stock consumption snapshot and movement; catalog-only products without stock history remain untracked.
- Reconciliation theoretical use is independently aggregated from those snapshots, with voids excluded and refunds applied proportionally, rather than using inventory consumption movements as its source of truth.
- The API reports snapshot coverage. Sales made before this migration do not have historical recipe snapshots and cannot be reconstructed reliably from current recipe definitions; those periods must be marked incomplete rather than silently backfilled from mutable recipes. Direct-sale products that have never had branch stock history are not included in COGS/stock usage until an opening stock movement establishes tracking.

### Reports

- `GET /api/v1/reports/summary` supports `sales`, `profit`, `stock-value`, `staff` and `category-mix` tabs with branch and date filters and bounded result sets.
- Sales includes gross/refunded/net sales, order count, average basket and daily sales.
- Profit includes net sales, COGS from the valuation ledger, approved/paid operating expenses, gross profit and profit after expenses.
- Staff performance aggregates order count and sales by staff account.
- Category mix aggregates sale-line value by product category.
- The Reports screen exposes all five tabs and CSV export.

## Verified source-level checks in this environment

- 50 numbered SQL migrations are present and CI's expected migration count is updated to 50.
- OpenAPI JSON parses and includes the new valuation, reporting, integration-settings and stock-count workflow routes.
- Go source has been formatted with `gofmt`.
- Inventory/procurement API smoke script passes `sh -n` syntax validation and now contains weighted-average valuation and draft-edit/submit/approve acceptance checks.
- Frontend TSX syntax transpilation passes for Inventory, Stock Taking, Reconciliation, Reports, registration, email verification, login, Google sign-in, password recovery and MFA pages.
- Existing offline POS Node tests pass (13/13).

## Implemented but not yet production-verified

### Authentication

Implemented in migration `050_auth_verification_recovery_mfa.sql` and `auth_flows.go`:

- Registration creates the workspace but does not issue a session; it sends a one-time email code and requires verification before password login.
- Verification and password-reset codes are stored as HMAC-SHA256 digests keyed by `JWT_SECRET`, expire after 15 minutes, are single-use, allow at most ten attempts and enforce a 60-second resend cooldown.
- Forgot/reset-password pages and APIs are implemented. Password reset revokes existing sessions.
- Optional TOTP MFA setup, enable, login verification and disable are implemented. TOTP secrets are encrypted with AES-256-GCM using `AUTH_ENCRYPTION_KEY`.
- Google Identity Services credential sign-in validates the ID token with Google's tokeninfo endpoint, checks audience, issuer and verified email, and supports existing registered accounts. Google-based first-time account/workspace creation and account linking are not implemented.

Required deployment configuration: `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME` (or `SMTP_USER`), `SMTP_PASSWORD`, `SMTP_FROM`, `JWT_SECRET` (at least 32 high-entropy characters), `AUTH_ENCRYPTION_KEY` (at least 32 high-entropy characters), `GOOGLE_CLIENT_ID` and `NEXT_PUBLIC_GOOGLE_CLIENT_ID`. Email and Google end-to-end flows still require real sandbox/deployment configuration and browser acceptance. Losing `AUTH_ENCRYPTION_KEY` makes existing MFA secrets unrecoverable; key rotation requires re-encryption.

### Integrations

- Daraja/M-Pesa provider code exists for OAuth, STK initiation, callbacks and settlement/reconciliation. Provider credentials and production callback configuration are deployment requirements.
- The specification describes eTIMS as configuration/status only and invoices are not transmitted to KRA; no eTIMS transmission implementation should be represented as complete.
- Card payments are recorded with a reference; no card gateway is wired.
- A per-business IntegrationSettings metadata/status screen and API now exist. It stores only non-secret public fields; M-Pesa live status requires deployment-level Daraja credentials, while eTIMS and card cannot be marked live because invoice transmission and gateway processing are intentionally not wired. Current Daraja runtime credentials are deployment-wide, not isolated per business, so per-business credential isolation remains a deployment/integration design limitation.

### Production acceptance

The full release gates remain unexecuted in this environment:

- Go 1.24.13 unit/integration tests, race tests, vet and production build. The local Go 1.23.2 attempted to download Go 1.24.13, but network DNS access to `proxy.golang.org` is unavailable.
- PostgreSQL 17 migrations, RLS checks, valuation trigger replay, authenticated inventory/procurement smoke tests and all database integration suites. No `psql`, PostgreSQL server or Docker runtime is installed here.
- Full Next.js install, lint and production build. Project dependencies are not installed in this workspace.
- Authenticated browser acceptance across the required screen flows.

Do not mark the specification production-complete until per-business Daraja credential isolation and any remaining integration scope are resolved, Google/email/MFA flows are tested with real providers, and all release gates above pass in the supported CI/deployment environment.
