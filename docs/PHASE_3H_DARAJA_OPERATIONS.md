# Phase 3H — Daraja operational visibility and review

This increment adds a tenant/branch-scoped operational queue for M-Pesa attempts and audit-only investigation notes.

## API

- `GET /api/v1/payments/mpesa/?status=&limit=&offset=` lists attempts for the authenticated selected branch. Phone numbers are masked; callback payloads and credentials are not returned.
- `POST /api/v1/payments/mpesa/{attemptID}/review-notes` records a bounded operator note for an attempt in `review_required`. It does not change payment, sale, or inventory state.
- `POST /api/v1/payments/mpesa/{attemptID}/reconcile` can query Safaricom for review-required attempts when both provider request identifiers are already stored. It never sends another STK Push.

## Safety boundaries

- An ambiguous STK submission without persisted provider identifiers remains under review and cannot be retried from this API.
- Review notes are audit records, not approval or settlement instructions.
- Provider reconciliation remains separate from atomic settlement. Only `provider_confirmed` attempts can enter the settlement handler.
- The list is limited to the selected tenant and branch and does not expose full phone numbers or raw callback JSON.

## Verification status

Formatting, source-level OpenAPI/CI parsing and migration static checks are run for this archive. Go tests and PostgreSQL 17 smoke tests require the pinned Go 1.24.13 and PostgreSQL 17 environment and are not represented as locally executed.
