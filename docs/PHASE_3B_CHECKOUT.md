# MEKU-In Phase 3B — Transactional cash checkout

## Delivered

- `POST /api/v1/sales/checkout` requires `pos.use` and an `Idempotency-Key` (8–64 characters).
- The server reloads every product's active/sellable status and current catalogue price. Client prices/totals are not accepted.
- Creates the sale, immutable product/SKU/name/quantity/price line snapshots, cash payment record, idempotency record and audit event within the request's tenant transaction.
- Advisory transaction lock serializes same-key requests within a business/branch. A matching replay returns the original sale; reuse with a different request returns 409.
- Only cash is accepted in this increment. M-Pesa/card are rejected until provider callback verification is implemented; a reference is never treated as settlement proof.
- Migration 016 adds sale lines, payments and branch-scoped checkout idempotency with forced tenant RLS and immutable line/payment rows.

## Deliberate boundaries

- Checkout does **not** yet deduct recipe ingredients. Recipe ingredient quantities are stored in units that may differ from product base units; the current schema validates dimensions but does not guarantee a conversion path or define a stock valuation/negative-stock policy. Deducting without resolving those semantics would risk incorrect stock balances.
- VAT, service charge, discount order and rounding remain unconfigured. Checkout uses catalogue line totals and sets total equal to subtotal, consistent with Phase 3A's explicit quote policy.
- Tables, tabs, held orders, split tenders, M-Pesa/card verification and refunds settlement remain later selling increments.

## Acceptance coverage added

The HTTP CI smoke test creates a cash checkout, verifies the server total and payment state, repeats the identical request with the same key and verifies the same sale is returned, then reuses the key with a different body and expects HTTP 409.

## Runtime verification

The PostgreSQL migration count in CI is updated to 16. Go tests, race tests, vet/build, PostgreSQL 17 execution and Next.js build still need an actual CI run; source changes and test scripts are not represented as passing runtime tests.
