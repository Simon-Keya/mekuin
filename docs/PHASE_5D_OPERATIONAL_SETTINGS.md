# Phase 5D — Branch Operational Settings

## Delivered

- Branch-scoped operational settings with forced tenant RLS.
- Manager/owner settings read and write permissions; other roles receive no settings access by default.
- Independent enable/disable controls for low-stock, out-of-stock, expense approval, supplier invoice review and cash variance alerts.
- Configurable cash variance critical threshold in integer minor currency units. Zero preserves the previous behaviour: every non-zero variance is a warning. A positive threshold marks absolute variances at or above the threshold critical.
- Existing product reorder levels remain the source of low-stock thresholds; no global stock quantity policy is imposed.
- Settings changes append an audit log event.
- `/settings` branch settings screen and workspace navigation.
- API: `GET/PUT /api/v1/settings/operational/`.

## Defaults and boundaries

A branch without a settings row behaves exactly as before: all alert types are enabled and cash variance threshold is zero. Settings are branch-scoped. The threshold uses minor units (KES 100 = KES 1.00). This phase does not change taxes, receipt numbering, payment provider credentials, or accounting policy.

## Verification

Local source-level checks only; PostgreSQL 17 migration/RLS smoke, Go 1.24.13 tests/build, Next.js production build and authenticated staging acceptance remain required CI/staging gates.
