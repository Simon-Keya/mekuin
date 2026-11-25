# Phase 4B — Procurement Frontend Increment

## Delivered

A first operational purchasing screen is available at `/purchasing`.

- Purchase order register with search and status summary.
- Loads purchase orders, active suppliers, and active products from the existing authenticated API.
- Creates draft purchase orders with supplier, PO number, notes, and multiple product lines.
- Opens purchase order details with ordered, received, and remaining quantities.
- Marks draft orders as ordered.
- Records partial or complete deliveries through the receiving endpoint, supplying a fresh idempotency key per submission.
- Shows receipt history returned by the detail endpoint.
- Offers cancellation only for draft and ordered states; the API remains authoritative for lifecycle validation.
- Adds a permission-aware entry point from the operations workspace.

## API dependencies

- `GET /api/v1/purchasing/orders`
- `POST /api/v1/purchasing/orders`
- `GET /api/v1/purchasing/orders/{orderID}`
- `POST /api/v1/purchasing/orders/{orderID}/order`
- `POST /api/v1/purchasing/orders/{orderID}/cancel`
- `POST /api/v1/purchasing/orders/{orderID}/receive`
- `GET /api/v1/catalog/suppliers`
- `GET /api/v1/catalog/products`

The page uses cookie credentials and the configured `NEXT_PUBLIC_API_BASE_URL`. It does not use mock data or write inventory directly. Inventory effects occur through the receiving API.

## Important limits

- This is an initial procurement workflow, not a complete purchasing/accounting module.
- Supplier invoice matching, payable balances, purchase returns, and supplier payments are not included.
- Receipt detail currently displays the receipt header history exposed by the API; per-receipt line breakdown is not yet exposed by the detail endpoint.
- No browser end-to-end test or live database test was run in this environment.
- `tsc --noEmit` could not validate the application because frontend dependencies are not installed. An attempted dependency install timed out. The errors reported by TypeScript were primarily missing Next.js/React/type packages, so this is not a successful build verification.

## Manual acceptance checks

1. Sign in with a user holding `purchasing.manage`, `suppliers.read`, and `products.read` and select a branch.
2. Open Operations workspace and use the Purchasing link.
3. Confirm real purchase orders, active suppliers, and active products load.
4. Create a draft order with two distinct products and confirm it appears in the register.
5. Open it and verify supplier, quantities, and line costs.
6. Mark it ordered and confirm the status changes.
7. Receive part of one line and confirm remaining quantity decreases and inventory movement is recorded.
8. Receive the balance and confirm the order reaches received state.
9. Confirm receipt history appears after refresh.
10. Confirm cancellation is unavailable for partially received or received orders and that API-side restrictions remain enforced.
11. Test with a user lacking purchasing permission and confirm the entry point is hidden and API access is denied.
