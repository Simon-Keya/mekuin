# Phase 5A — Cash shifts and drawer reconciliation

## Delivered

- `GET /api/v1/shifts/current`: current authenticated user's open shift and live expected drawer amount.
- `POST /api/v1/shifts/`: open a branch/user shift with a non-negative opening float. A partial unique index permits only one open shift per user and branch.
- `POST /api/v1/shifts/{shiftID}/movements`: immutable, reasoned `paid_in` / `paid_out` cash drawer movements.
- `POST /api/v1/shifts/{shiftID}/close`: lock and close the shift, calculate expected cash, store counted cash and variance, and append an audit event in one transaction.
- `GET /api/v1/shifts/{shiftID}/summary`: retrieve a closed shift summary suitable for a print sheet.
- Cash checkout now requires an open shift and attaches each cash tender to that shift. M-Pesa/card tenders do not affect drawer cash.

## Cash calculation

`expected = opening float + cash tenders assigned to this shift + paid-in - paid-out`

`variance = counted cash - expected`

All amounts are integer minor units. The shift is locked during movement recording and close; checkout also locks the active shift before attaching cash tenders. This serializes cash tender attribution against close. A failed checkout/close rolls back all related changes.

## Access

Cashiers and bartenders can manage their own shifts. Managers can inspect/close shifts for their branch through the same endpoints; ownership checks are enforced in handlers. Owner retains wildcard access. Stock-controller has no shift permission.

## Explicit accounting boundaries

- Refund records do not currently identify whether the refund was paid in cash, so they are not silently subtracted from expected cash. A cash refund must be represented by a reasoned paid-out movement until refund tender attribution is implemented.
- Supplier payments remain outside drawer movements. Phase 5B now links approved cash operating-expense payments to exactly one paid-out movement in the same transaction.
- No shift reopening, forced close, cross-branch close or currency conversion is supported.
- Alerts and audit-ledger UI remain later Phase 5 increments.

## Acceptance scenarios

1. Open a shift with KES 2,000 float; a second open attempt for the same user/branch returns 409.
2. Cash checkout without an open shift returns 409 `cash_shift_required` and creates no sale/payment.
3. Cash tender of KES 1,500 is attributed to the open shift; M-Pesa tender is not included.
4. Paid-in KES 300 and paid-out KES 100 result in expected cash KES 3,700.
5. Counted cash KES 3,650 closes with variance -KES 50.
6. Concurrent close attempts produce exactly one successful close; cash movements cannot be added after close.
7. Another tenant/branch cannot read or mutate the shift; cashier cannot close another user's shift.
8. Runtime role cannot delete shifts or update/delete movement history.
