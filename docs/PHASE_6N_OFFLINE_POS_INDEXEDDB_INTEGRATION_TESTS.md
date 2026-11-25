# Phase 6N — Offline POS IndexedDB integration tests

## Scope

Phase 6N adds an IndexedDB-backed integration test suite for the browser outbox and local POS caches. The suite uses `fake-indexeddb` so the production storage module is exercised against IndexedDB request/transaction semantics in Node, rather than testing only pure policy helpers.

## Automated scenarios

- Draft persistence, diagnostic initialisation and business/branch-filtered listing.
- Concurrent atomic claims: two simultaneous claim attempts must yield one successful claimant.
- Blocked and synced drafts cannot be claimed.
- Concurrent state updates retain bounded diagnostic history.
- Stale interrupted-sync recovery preserves the original client reference and attempt count.
- Product, basket, table and active-context cache round trips, including branch isolation and inactive-table filtering.
- Targeted outbox deletion leaves unrelated drafts intact.

## Running

From `apps/web`:

```bash
npm install
npm run test:offline
```

The script runs the existing pure policy suite followed by the IndexedDB integration suite. Node 22+ is required for the built-in TypeScript type-stripping flag used by the second test command.

## Boundaries

`fake-indexeddb` exercises IndexedDB API behaviour but is not a substitute for Chromium/Firefox/Safari testing, real browser quota behaviour, service-worker lifecycle testing, or multi-device coordination. The concurrent-claim test validates the atomic IndexedDB claim path within one shared database. It does not prove network-level exactly-once delivery; server idempotency remains mandatory.

## Verification record

The pure policy suite and source syntax checks are run in the implementation environment. The new IndexedDB suite requires installing the added `fake-indexeddb` development dependency; where dependencies are not installed in the implementation environment, it is reported as pending rather than represented as passed.
