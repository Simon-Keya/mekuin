# Phase 6M — Offline POS automated policy tests

## Implemented

- Extracted the pure stale-sync and claim eligibility rules into `src/lib/offline-pos-policy.mjs`, keeping the production IndexedDB code and tests on the same policy implementation.
- Centralised bounded diagnostic-history appends in the same module.
- Added Node's built-in test-runner suite at `tests/offline-pos-policy.test.cjs`.
- Added `npm run test:offline` to run the suite without installing a browser test framework.

## Automated coverage

- Pending drafts are claimable.
- Synced and blocked drafts are never claimable.
- Recent syncing drafts are protected from another tab.
- A syncing draft at the stale threshold can be reclaimed.
- Invalid timestamps are recoverable; future timestamps are treated as recent.
- Diagnostic history retains only the newest 30 entries and handles malformed history/limits.

## Verification boundary

The policy suite exercises pure decision logic. It does **not** simulate IndexedDB transactions, browser tabs, service workers, network interruption, API authentication, PostgreSQL, or end-to-end POS flows. Those still require browser/integration acceptance. No payment, checkout, stock movement or completed-sale capability was added to offline mode.
