# Phase 4B — Receiving API contract (next implementation slice)

This contract is the target for the transactional receiving handler. It is not an assertion that the handlers are already wired.

## Routes
- `GET /api/v1/purchasing/orders?status=&q=&limit=&offset=` — tenant/branch scoped listing.
- `POST /api/v1/purchasing/orders` — create a draft order with line items.
- `GET /api/v1/purchasing/orders/{orderID}` — order details and receipt history.
- `POST /api/v1/purchasing/orders/{orderID}/order` — draft to ordered.
- `POST /api/v1/purchasing/orders/{orderID}/receive` — receive quantities against lines.

## Receive request
Header: `Idempotency-Key` (required, 8–128 printable characters).
Body:
```json
{
  "note": "Delivery note optional",
  "lines": [{"purchase_order_line_id":"uuid","quantity":"2.5"}]
}
```

## Receiving transaction requirements
1. Validate request, key and fingerprint.
2. Lock the PO row, then product stock advisory locks in sorted product-ID order.
3. Verify PO belongs to active business and branch and is ordered or partially received.
4. Verify active product and line ownership; reject duplicate line IDs, non-positive quantities, malformed decimal quantities and quantities beyond outstanding ordered quantity.
5. If key exists with identical fingerprint, return existing receipt; if key exists with different fingerprint, return conflict.
6. Insert receipt header and receipt lines, append positive `purchase_receipt` movements, update received quantities and PO status, insert audit record.
7. Commit as one tenant transaction; no partial posting.

## Response
201 for first successful receipt, 200 for identical idempotent replay; include receipt ID, PO ID, status, and posted line summary. Conflict responses should distinguish replay-key mismatch, over-receipt, and invalid PO lifecycle.

## Acceptance scenarios
- Draft order cannot be received.
- Full receipt and multiple partial receipts update lifecycle correctly.
- Over-receipt is rejected and leaves no ledger changes.
- Identical replay is idempotent; mismatched replay key payload is rejected.
- Concurrent receipts cannot over-receive.
- Cross-tenant and unassigned-branch requests are rejected.
- Invalid line causes complete rollback.
