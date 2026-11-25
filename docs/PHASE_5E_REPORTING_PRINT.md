# Phase 5E — Operational Reporting and Print Layouts

## Delivered

- Date-filtered closed cash-shift history API (`GET /api/v1/shifts/history`).
- Cash-shift history is branch-wide for users with `shifts.manage`; other permitted shift users see only their own shifts.
- Date-filtered operational summary API (`GET /api/v1/reports/operational-summary`). Expense totals are grouped by currency and workflow status; no currencies are converted or combined.
- `/reports` screen with expense totals, closed shift reconciliation columns, date filters, refresh and browser print layout.
- Workspace link gated by `expenses.read`.
- OpenAPI paths and API description updated.

## Reporting rules and boundaries

- Dates filter expense date and shift close date respectively.
- Shift history is limited to the 100 most recent closed shifts in the requested period; default is 50.
- Staff without `shifts.manage` see their own shifts only.
- Money remains in integer minor units. Currency totals remain separate; no FX conversion is performed.
- The report is an operational summary, not a statutory financial statement, bank reconciliation, tax report, or accounting ledger.
- Print uses the browser print dialog; PDF generation/download is controlled by the browser.

## Verification

Source-level checks include Go formatting, OpenAPI JSON parsing, CI YAML parsing, TypeScript TSX syntax transpilation and ZIP integrity. Go 1.24.13 tests/build, PostgreSQL 17 migration/RLS execution, Next.js lint/production build and authenticated staging acceptance remain external gates.
