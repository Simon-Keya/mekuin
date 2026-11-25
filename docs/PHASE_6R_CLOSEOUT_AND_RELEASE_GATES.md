# Phase 6R — Phase 6 implementation closeout and release gates

## Source implementation closeout

The Phase 6 offline POS technical foundation is implemented through 6Q: durable business/branch-scoped IndexedDB draft storage, stable client references and server idempotency, replay reconciliation, tenant-scope checks, atomic claims and diagnostics, stale recovery, cached catalogue/table context, service-worker cache boundaries, operator inspection/correction, automated policy and service-worker tests, and browser storage safeguards.

## Required release gates (not claimed complete)

1. Install dependencies in `apps/web` and run `npm run test:offline`; the IndexedDB suite requires `fake-indexeddb`.
2. Run the full TypeScript check, ESLint and production Next.js build.
3. Run Go tests, race tests, vet/build and PostgreSQL 17 migration/RLS integration checks.
4. Run authenticated staging acceptance with real business/branch memberships and POS permissions.
5. In real browsers, test first online visit, offline reload, airplane-mode draft capture, reconnect, lost response after server commit, two-tab claim contention, stale recovery, tenant/branch switch, blocked correction, quota errors and user-cleared storage.
6. Verify no API/auth/tenant response is served from Cache Storage.

## Explicitly deferred decisions

- Offline cash/card/mobile-money tender and payment verification.
- Shift attribution, cash-up and reconciliation for any offline tender.
- Offline discount, void, refund and manager-approval authority.
- Business policy for repriced/deactivated products, stock changes and table occupancy conflicts.
- Recovery expectations after browser storage eviction or user-cleared site data.

## Release status

Phase 6 source implementation is closed for the conservative order-draft-only scope. It is not production accepted until all external release gates above pass and the business decisions required for any expanded offline action are approved.
