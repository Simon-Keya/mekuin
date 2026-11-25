# Phase 5F — Report exports and Phase 5 acceptance

## Delivered in this increment

- Added CSV export for the cash-shift history currently loaded by `/reports`.
- Added CSV export for expense totals currently loaded by `/reports`, grouped by currency and workflow status.
- CSV values are quoted and embedded quotes are escaped. Values beginning with spreadsheet formula prefixes (`=`, `+`, `-`, `@`) after leading ASCII whitespace/control characters are prefixed with an apostrophe to reduce CSV formula-injection risk.
- Exports use the selected report date range in their filenames and include a UTF-8 BOM for common spreadsheet compatibility.
- Existing server-side tenant, branch and permission scoping remains authoritative; the browser exports only the rows returned by the API.

## Explicit report limitations

- Cash-shift history is capped at the 100 most recently closed shifts in the selected date range. CSV export contains only those currently loaded rows; it is not an exhaustive export when more than 100 shifts match.
- Expense CSV contains aggregate currency/status totals, not individual expense rows.
- No currencies are converted or combined.
- These reports are operational summaries, not statutory accounts, tax filings, bank reconciliations or a general ledger.
- Scheduled delivery, background exports, and server-generated PDF files are not included.

## Phase 5 consolidated acceptance checklist

### Source implementation
- [x] 5A: branch/user cash shifts, cash tender attribution, immutable drawer movements and close variance.
- [x] 5B: expense categories, expense workflow, approval/rejection and atomic cash expense payment integration.
- [x] 5C: operational alerts, acknowledgement/resolution and audit ledger browsing.
- [x] 5D: branch operational settings and configurable cash variance critical threshold.
- [x] 5E: date-filtered operational reports and print-friendly reporting page.
- [x] 5F: CSV export for currently loaded shift history and expense summary data.

### External release gates — not yet certified
- [ ] Run all migrations 001–039 against PostgreSQL 17 from a clean database.
- [ ] Execute every database RLS/privilege smoke test in CI.
- [ ] Run `go test ./...`, `go test -race ./...`, `go vet ./...` and production API build with Go 1.24.13.
- [ ] Run web lint and production build using the lockfile-pinned dependencies.
- [ ] Run authenticated staging acceptance for each role, branch boundary and tenant boundary.
- [ ] Verify cash checkout requires an open shift and cash expense payment reduces drawer expected cash exactly once.
- [ ] Verify expense creator self-approval is rejected and manager approval/rejection is audited.
- [ ] Verify alert acknowledgement/resolution, settings permissions and audit ledger visibility.
- [ ] Reconcile report totals against source records, test date boundaries, and confirm currency separation.
- [ ] Confirm CSV exports contain only the current user's authorized scope and document the 100-shift limit.
- [x] Harden CSV formula-prefix detection for formula operators preceded by leading ASCII whitespace/control characters (v67).

Phase 5 source implementation is considered feature-complete at this increment, but Phase 5 is **not production-accepted** until all external release gates above pass. Any failed gate should be fixed and re-run before Phase 6 begins.
