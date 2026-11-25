# Phase 4B — next acceptance checklist

Run against a disposable PostgreSQL database after applying migrations through 031:

1. Create a draft PO with one supplier and multiple active products; verify duplicate product lines are rejected.
2. Fetch PO detail and verify ordered, received and remaining quantities plus an empty receipt list.
3. Place the PO; verify draft-only transition and audit record.
4. Receive part of one line; verify one receipt, one receipt line, one `purchase_receipt` inventory movement, and `partially_received` status.
5. Replay the same idempotency key and identical payload; verify the original receipt ID is returned and no additional movement is created.
6. Replay the same key with a different payload; verify HTTP 409 and no writes.
7. Attempt to receive more than the outstanding quantity; verify HTTP 409 and transaction rollback (no receipt or movement).
8. Receive the remaining quantity; verify `received` status and zero remaining quantity.
9. Attempt cancellation after any receipt; verify HTTP 409.
10. Cancel a draft PO and an ordered PO with no receipts; verify status and audit event.
11. Attempt detail, cancel, order and receive from another business and another branch; verify no cross-tenant or cross-branch access.
12. Race two receives against the same remaining quantity; verify the total posted quantity never exceeds the ordered quantity.
13. Verify database runtime role can update only the explicitly granted lifecycle/received columns and cannot delete procurement or ledger rows.

Do not mark Phase 4B production-ready until these checks pass on PostgreSQL and the full Go test suite passes with the pinned Go toolchain.
