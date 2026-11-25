# Phase 8E — CI and platform audit privilege hardening

## Changes

- Updated the CI migration-state assertion from 39 to 42, matching the repository's 42 ordered Goose migration files. The previous assertion would fail after migrations 040–042 had been applied successfully.
- Added `db/tests/platform_admin_smoke.sql` to verify that the platform audit table exists, the application runtime role can read and append audit events, the runtime role cannot mutate or truncate historical events, and the expected audit lookup indexes exist.
- Added the platform privilege smoke test to the PostgreSQL 17 CI job after the runtime role is provisioned.
- Corrected the README's stale Phase 1-only implementation summary so it reflects the feature areas present in the current repository and clearly retains the production acceptance gates.

## Verification in this environment

- Offline POS policy and service-worker tests: 13 passed.
- CI migration-count consistency check: passed (42 migration files; CI expects 42).
- Go formatting check: changed Go sources are gofmt-clean; no Go source changes were required in this increment.
- IndexedDB integration test: not run successfully because `fake-indexeddb` is not installed in this extracted archive.
- Go tests/race/vet/build: not run because this environment has Go 1.23.2 while the repository pins Go 1.24.13.
- PostgreSQL migration and privilege smoke tests: not run because `psql`/PostgreSQL are unavailable in this environment.
- Next.js lint/production build and authenticated browser acceptance: not run because frontend dependencies are not installed.

## Release boundary

This increment improves CI correctness and adds a platform audit privilege gate. It does not establish production readiness. The remaining external gates above must pass against PostgreSQL 17, Go 1.24.13, a dependency-installed Next.js environment, and an authenticated deployed/staging environment.
