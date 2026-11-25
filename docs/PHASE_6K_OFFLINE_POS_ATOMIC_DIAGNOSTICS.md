# Phase 6K — Atomic offline POS diagnostics

## Problem
Phase 6J added a bounded local timeline, but `updateDraft` read the previous record in a separate readonly transaction and wrote the new record in a later readwrite transaction. Two tabs updating the same draft could both read the same old history and then overwrite one another, losing a diagnostic event.

## Implementation
- Changed `updateDraft` to read the existing outbox record, derive the transition history, and write the updated draft within one IndexedDB `readwrite` transaction.
- Retained the latest 30 history events per draft.
- Kept compatibility with existing records that have no `history` field and with drafts supplied with an initial history.
- Added explicit IndexedDB request, transaction error and abort handling for the atomic update.

## Safety and scope
- The change is local to browser outbox persistence; no API contract, database migration, permissions, pricing, checkout, payments, inventory or completed-sale behaviour changes.
- The stable `client_reference` and server idempotency behaviour are unchanged.
- IndexedDB transaction atomicity prevents the read/modify/write lost-update window within this operation. It does not turn browser diagnostics into a server audit log or provide a distributed sync lease.

## Acceptance checks
1. Update the same draft from two tabs in close succession and verify the stored state remains valid and history is bounded to 30 entries.
2. Race a `syncing` transition against a `synced` transition and verify the final persisted record has the server order ID and a diagnostic event for the successful confirmation.
3. Load legacy records without `history` and verify they remain readable and can be updated.
4. Simulate IndexedDB transaction abort/quota failure and verify the caller receives an error rather than showing a successful local update.
5. Confirm history remains local-only and is not included in API create/reconcile payloads.
