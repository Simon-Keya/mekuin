# Phase 6Q — Offline action boundary and operational recovery

The supported offline action is local POS order-draft capture and later server replay. A local draft is not a sale and is not evidence of payment.

The following remain online-only: checkout completion, payment recording/verification, revenue recognition, stock deduction, cash-shift movement, discounts, voids, refunds, and manager approvals. Cached catalogue prices and cached table lists are indicative only; the server revalidates current product, price, table, tenant and permission state during replay.

A stale or invalid draft may be blocked for operator correction. Correction creates a new client reference; retrying an unchanged queued draft preserves its original reference. A browser may evict or clear local storage. Persistent-storage requests are best-effort and do not constitute backup.

No unresolved financial or approval policy has been inferred in this phase. Offline cash and completed sales require explicit product, accounting, shift, approval, audit and reconciliation decisions before implementation.
