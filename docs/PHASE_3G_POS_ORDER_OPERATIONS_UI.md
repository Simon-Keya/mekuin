# MEKU-In Phase 3G — POS order operations UI

## Delivered

The authenticated `/orders` page lists the active open/held orders returned by the existing tenant- and branch-scoped API. Operators can inspect order snapshots, adjust quantities, save line replacements, hold an open order, reopen a held order, cancel an uncompleted draft, or proceed to cash checkout.

Line edits send only product IDs and quantities. The API reloads catalogue prices and replaces the server-side line snapshots; browser-calculated totals are not trusted. Checkout uses a fresh `Idempotency-Key` and the existing order checkout route, which delegates to the transactional sale implementation. The UI does not create a payment or stock movement when saving, holding or cancelling a draft.

Cash checkout remains subject to the server's active shift, inventory and recipe validation. The UI deliberately does not offer card or unverified M-Pesa checkout, tax/service-charge calculations, or discounts. Those require the relevant approved policy and provider verification flows.

## Routes used

- `GET /api/v1/sales/orders`
- `GET /api/v1/sales/orders/{orderID}`
- `PUT /api/v1/sales/orders/{orderID}`
- `POST /api/v1/sales/orders/{orderID}/hold`
- `POST /api/v1/sales/orders/{orderID}/reopen`
- `POST /api/v1/sales/orders/{orderID}/cancel`
- `POST /api/v1/sales/orders/{orderID}/checkout`

## Acceptance still required

Run authenticated browser tests for list/detail/edit/hold/reopen/cancel, duplicate checkout replay, cash shift enforcement, recipe stock deduction and tenant/branch isolation. Also run the pinned Go suite, PostgreSQL 17 migrations/RLS smoke tests and Next.js production build before release.
