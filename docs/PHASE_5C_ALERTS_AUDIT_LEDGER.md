# Phase 5C — Operational Alerts and Audit Ledger

## Delivered

- Durable, tenant- and branch-scoped operational alert records with open, acknowledged and resolved states.
- Alert refresh on `GET /api/v1/alerts/`. The source tables remain authoritative; this endpoint materializes current conditions and resolves stale alerts.
- Low-stock and out-of-stock alerts for active products that have inventory movements in the selected branch. This avoids treating never-stocked catalogue products as zero-stock alerts.
- Pending expense approval and supplier-invoice review alerts.
- Cash-shift discrepancy alerts for closed shifts with non-zero variance.
- Alert acknowledgement and resolution actions, append-only alert event history, and corresponding audit-log entries.
- Business-wide, append-only audit ledger browsing with action, entity-type and free-text filters.
- Workspace links and dedicated `/alerts` and `/audit` pages.
- Forced tenant RLS, branch scoping, active-alert deduplication and least-privilege runtime grants.

## API

- `GET /api/v1/alerts/?status=open|acknowledged|resolved|all`
- `POST /api/v1/alerts/{alertID}/action` with `{ "action": "acknowledge" | "resolve", "note": "..." }`
- `GET /api/v1/audit/?limit=100&offset=0&q=...&action=...&entity_type=...`

Alerts are evaluated when the alert inbox is loaded. This increment does not add a background scheduler or push notifications. Alert refresh is bounded to the selected branch. Alert resolution occurs automatically when the underlying source condition clears; manual resolution is also available to managers and owners.

## Alert rules and boundaries

- Stock alerts require at least one inventory movement for the product in the selected branch. `stock_out` is on-hand quantity less than or equal to zero. `low_stock` is positive on-hand quantity at or below the product reorder level.
- Expense alerts are created for expenses in `submitted` status.
- Supplier invoice alerts are created when the latest review event is `submitted` and the invoice is not paid or void.
- Cash variance alerts are created for closed shifts whose stored variance is non-zero. No arbitrary materiality threshold is imposed; severity remains warning until a business-configurable threshold is approved.
- No generic overdue-task alert is introduced because the project has no agreed SLA/overdue thresholds for approvals or other tasks.
- Audit browsing is business-wide because the existing immutable `audit_logs` schema has no branch identifier. Branch-specific audit filtering requires a deliberate schema expansion and historical attribution strategy.
- The audit ledger exposes metadata already recorded by existing operations; it does not invent missing actor, branch, or source details.

## Verification

Source checks include Go formatting/syntax parsing, OpenAPI JSON and route-contract validation, CI YAML parsing, migration-count consistency, frontend TSX transpilation and ZIP integrity. Full Go 1.24.13 tests/build, PostgreSQL 17 migration/RLS smoke tests, and Next.js dependency-backed lint/build remain CI/staging gates because the local environment does not have the pinned Go toolchain, PostgreSQL client/server, or installed frontend dependencies.
