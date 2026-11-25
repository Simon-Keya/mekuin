# MEKU-In repair report — 2026-11-25

## Backend

| Defect | Resolution |
|--------|------------|
| Missing `revokeStaffInvitation` | Implemented with tenant scope, pending-only revoke, audit log |
| Purchasing `replayed: true` type | `map[string]any` response |
| `sales_checkout` undefined `err` | `:=` on order lock query |
| `staff` undefined `err` | `:=` on branch audit |
| Unused `errors` import | Removed |
| Expense reject length vs unit test | Min 10 trimmed chars on reject |
| Daraja phone mask vs unit test | Prefix 3/+4, stars=`len-7` |
| OpenAPI test path-level parameters | RawMessage path items; optional JSON error bodies |

Verified: `go build ./cmd/api`, `go test ./cmd/api` — PASS.

## Frontend

| Defect | Resolution |
|--------|------------|
| Payables unused destructure | Safe delete of optional PO line id |
| Platform anchor to `/` | `next/link` |
| POS storage status state missing | Typed `useState` for OfflineStorageStatus |
| Settings `any` | Capabilities types |
| Shifts unused `viewSummary` | Removed (close already loads summary) |
| offline-pos-policy.mjs types | Added `offline-pos-policy.ts` |

## Docker / ops

- API Dockerfile copies `go.sum`
- Web Dockerfile: build-arg API URL, non-root, standalone
- `docs/RENDER_DEPLOYMENT.md` added

## Not verified here

- Docker daemon builds (no Docker in environment)
- Live Postgres migration apply
- Full multi-service integration against Render

Business financial/stock rules were not intentionally changed except phone-mask and expense-reason alignment with existing unit tests.
