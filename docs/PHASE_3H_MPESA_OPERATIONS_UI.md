# MEKU-In Phase 3H — M-Pesa payment operations UI

## Delivered

The `/payments` page lists the latest 100 M-Pesa attempts for the authenticated selected branch and filters by the server-supported attempt status. It displays amount/currency, masked phone, provider receipt/description, timestamps and a shortened sale reference. It does not expose full MSISDNs, callback payloads or provider credentials.

Operators can query Safaricom only when the API returns `can_reconcile`, and can request settlement only when the API returns `can_settle`. Settlement remains a backend-controlled atomic operation; provider confirmation alone does not complete the sale or stock deduction. Review-required attempts accept an audit note only, never a force-settlement action.

## Routes used

- `GET /api/v1/payments/mpesa?limit=100&status=...`
- `POST /api/v1/payments/mpesa/{attemptID}/reconcile`
- `POST /api/v1/payments/mpesa/{attemptID}/settle`
- `POST /api/v1/payments/mpesa/{attemptID}/review-notes`

## Acceptance still required

Validate with Daraja sandbox credentials and real callback delivery, including duplicate callbacks, provider query pending/success/failure, callback mismatch, missing callback metadata, replayed settlement, and stock shortage at settlement. Confirm secrets are server-only and callback ingress is publicly reachable with the provider's required TLS configuration.
