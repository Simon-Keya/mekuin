# Architecture decisions

- AD-001: Use Next.js App Router + Go/Chi + PostgreSQL. This supersedes Base44/TanStack Start in the supplied booklet.
- AD-002: Modular monolith, not microservices.
- AD-003: PostgreSQL is the system of record; stock ledger is authoritative and balances are derived projections.
- AD-004: Tenant identity comes from authenticated membership; RLS is defence in depth.
- AD-005: All monetary calculations will use integer minor units or exact decimal arithmetic, never floating point.
- AD-006: Offline POS is Phase 6 and must replay idempotently using client-generated references.
- AD-007: Unresolved tax, valuation and operational policies remain explicit decisions until approved.
