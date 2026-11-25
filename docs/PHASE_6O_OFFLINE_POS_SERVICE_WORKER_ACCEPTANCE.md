# Phase 6O — Offline POS service-worker acceptance

## Scope

This increment adds a deterministic Node test harness for the existing service worker and makes it part of `npm run test:offline`. It does not claim to replace real-browser acceptance.

## Automated coverage

- Service-worker install and activation handlers execute.
- API, authentication and tenant-context requests bypass service-worker interception and caching.
- Non-GET and cross-origin requests bypass interception.
- Static Next.js assets are fetched and cached when the network succeeds.
- An uncached `/pos` navigation while offline returns the explicit 503 fallback rather than pretending the POS shell is available.

## Run

From `apps/web`:

```bash
npm run test:offline
```

The command runs policy tests, service-worker lifecycle tests and IndexedDB integration tests. The IndexedDB suite requires the declared `fake-indexeddb` development dependency and a Node runtime supporting `--experimental-strip-types`.

## Required real-browser acceptance (not covered by Node harness)

1. Open `/pos` while authenticated and online; wait for service-worker control and catalogue/table cache completion.
2. Reload, then use browser DevTools Network > Offline. Confirm the cached POS shell loads and locally saved drafts remain visible.
3. Confirm API requests fail visibly while offline and no authenticated API response is served from Cache Storage.
4. Restore connectivity; verify reconciliation runs before create replay and server-confirmed drafts are not duplicated.
5. Open two tabs in the same profile; attempt to sync the same draft from both and verify one active claim.
6. Close a tab during a request, reopen after the stale threshold, and verify the same `client_reference` is retained.
7. Switch business/branch while online and verify drafts from the previous scope are not replayed in the new scope.
8. Clear site storage only in a disposable test account and document that browser storage eviction/user clearing can remove locally queued drafts.

## Limitations

- The Node harness uses mocked Cache Storage and fetch; it is not Chromium/Firefox/WebKit execution.
- Service-worker control timing, browser storage eviction, actual offline navigation, and multi-tab lifecycle remain browser acceptance items.
- Offline POS remains limited to order drafts. Payments, completed sales, stock deductions and cash movements are not supported offline.
