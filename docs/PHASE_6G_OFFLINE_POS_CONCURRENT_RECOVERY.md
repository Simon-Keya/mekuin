# Phase 6G — Offline POS concurrent recovery safeguards

## Problem addressed

A draft marked `syncing` may belong to another live browser tab. Earlier recovery treated every `syncing` record as abandoned during page initialization. That could reset a live request to `pending` while the first tab was still waiting for the server response. The server idempotency key protects against duplicate order creation, but concurrent local state writes could still overwrite a successful result or cause confusing operator feedback.

## Changes

- Recovery now considers a `syncing` record interrupted only when its `updated_at` timestamp is at least two minutes old (or invalid).
- Recent `syncing` records are left untouched so a second tab does not prematurely recover a live operation.
- The outbox exposes a **Recover stale** action. It applies the same age threshold, refreshes the queue, and reports whether anything was recovered.
- The stable `client_reference` is preserved during recovery. Replays continue to use the existing server idempotency contract.
- Existing Web Locks coordination and the per-tab mutex remain in place.

## Safety boundaries

- This is a client-side stale-record heuristic, not a distributed lease or server-side lock.
- A request taking longer than two minutes may be considered stale by another tab. Replaying remains safe only because the server idempotency implementation is authoritative and must remain enabled.
- Do not delete pending, syncing or blocked drafts automatically.
- Offline POS still creates order drafts only. It does not record payments, complete sales, deduct stock or move cash.

## Verification still required

Browser testing should keep two tabs open, delay a sync beyond normal response time, reload the second tab, and verify that recent `syncing` entries are not reset. Also test recovery of a genuinely stale record, timeout-after-commit, and repeated replay with the same client reference. Go/PostgreSQL integration, full Next.js build and authenticated staging checks remain external acceptance tasks.
