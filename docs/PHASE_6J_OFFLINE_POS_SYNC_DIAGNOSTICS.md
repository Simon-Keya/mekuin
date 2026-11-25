# Phase 6J — Offline POS sync diagnostics and history

## Problem
The outbox showed only the latest state and error. Operators could not easily distinguish an initial local save, an active send, an interrupted attempt, a server rejection, or a recovered server confirmation after several transitions.

## Implementation
- Added an optional, backward-compatible `history` array to locally stored outbox drafts. Existing IndexedDB records without history remain readable.
- New drafts receive an initial local-save event.
- Outbox state, attempt, error, or server-reference changes append a timestamped diagnostic event.
- History is bounded to the latest 30 events per draft to limit local storage growth.
- Expanded draft inspection now displays the recent state-transition timeline, message, timestamp and attempt number.
- Diagnostics remain local to the browser; no customer data or additional telemetry is transmitted.

## Safety
- No API, database schema, permission, checkout, payment, inventory or completed-sale behaviour is changed.
- Existing client references are preserved; diagnostics do not initiate retries or mutate server orders.
- Legacy drafts without history continue to display, with an explanatory empty-history message.
- History is bounded and is not a substitute for the server's immutable audit ledger.

## External acceptance
1. Save a new draft and verify its initial local-save event appears.
2. Sync successfully and verify `syncing` and `synced` transitions and the server order reference are visible.
3. Simulate a retryable network/5xx failure and verify the pending transition and attempt count are visible.
4. Trigger a definitive 4xx response and verify the blocked transition and error are visible.
5. Reconcile a timeout-after-commit and verify the recovery event appears without a second order creation.
6. Load a legacy IndexedDB draft without a `history` field and verify inspection still works.
7. Generate more than 30 transitions and verify only the latest 30 remain.
8. Confirm history is scoped with its draft and is not sent to the API.
