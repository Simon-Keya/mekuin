import 'fake-indexeddb/auto';
import test, { afterEach } from 'node:test';
import assert from 'node:assert/strict';
import {
  enqueueDraft, listDrafts, claimDraft, updateDraft, deleteDraft,
  cacheProducts, readCachedProducts, saveBasket, readSavedBasket,
  cacheTables, readCachedTables, cachePOSContext, readCachedPOSContext,
  recoverInterruptedDrafts,
} from '../src/lib/offline-pos.ts';

const business = 'business-test';
const branch = 'branch-test';
const reference = () => `offline-${crypto.randomUUID()}`;
function draft(overrides = {}) {
  const now = new Date().toISOString();
  return {
    client_reference: reference(), business_id: business, branch_id: branch,
    service_type: 'walk_in', table_id: '', label: 'Test draft',
    lines: [{ product_id: 'product-1', quantity: 2 }], state: 'pending',
    attempts: 0, last_error: '', server_order_id: '', created_at: now,
    updated_at: now, ...overrides,
  };
}

afterEach(async () => {
  await new Promise((resolve, reject) => {
    const req = indexedDB.deleteDatabase('meku-in-offline-pos');
    req.onsuccess = () => resolve(); req.onerror = () => reject(req.error);
    req.onblocked = () => reject(new Error('Database deletion blocked by an open connection'));
  });
});

test('outbox persists drafts and scopes listing to business and branch', async () => {
  const item = draft();
  await enqueueDraft(item);
  await enqueueDraft(draft({ business_id: 'other-business' }));
  const rows = await listDrafts(business, branch);
  assert.equal(rows.length, 1);
  assert.equal(rows[0].client_reference, item.client_reference);
  assert.equal(rows[0].history.length, 1);
});

test('concurrent claims grant a draft to only one caller', async () => {
  const item = draft();
  await enqueueDraft(item);
  const claims = await Promise.all([
    claimDraft(item.client_reference), claimDraft(item.client_reference),
  ]);
  assert.equal(claims.filter(Boolean).length, 1);
  const claimed = claims.find(Boolean);
  assert.equal(claimed.state, 'syncing');
  assert.equal(claimed.attempts, 1);
});

test('blocked and synced drafts cannot be claimed', async () => {
  for (const state of ['blocked', 'synced']) {
    const item = draft({ state }); await enqueueDraft(item);
    assert.equal(await claimDraft(item.client_reference), null);
  }
});

test('atomic updates preserve transition history and cap it at thirty events', async () => {
  const item = draft(); await enqueueDraft(item);
  const updates = Array.from({ length: 35 }, (_, i) => updateDraft({
    ...item, state: i % 2 ? 'pending' : 'syncing', attempts: i + 1,
    updated_at: new Date(Date.now() + i * 1000).toISOString(),
  }));
  await Promise.all(updates);
  const [stored] = await listDrafts(business, branch);
  assert.ok(stored.history.length <= 30);
  assert.ok(stored.history.length >= 1);
});

test('interrupted stale sync is recovered without changing its idempotency reference', async () => {
  const item = draft({ state: 'syncing', updated_at: new Date(Date.now() - 180_000).toISOString(), attempts: 2 });
  await enqueueDraft(item);
  assert.equal(await recoverInterruptedDrafts(business, branch), 1);
  const [stored] = await listDrafts(business, branch);
  assert.equal(stored.state, 'pending');
  assert.equal(stored.client_reference, item.client_reference);
  assert.equal(stored.attempts, 2);
});

test('product, basket, table and context caches round-trip with branch scope', async () => {
  const products = [{ id: 'p1', sku: 'SKU-1', name: 'Test', sale_price_minor: 1200, is_active: true, is_sellable: true }];
  const tables = [
    { id: 't1', table_code: 'T1', display_name: 'Table 1', capacity: 4, is_active: true },
    { id: 't2', table_code: 'T2', display_name: 'Table 2', capacity: 2, is_active: false },
  ];
  await cacheProducts(business, branch, products);
  await saveBasket(business, branch, [{ product_id: 'p1', quantity: 3 }]);
  await cacheTables(business, branch, tables);
  await cachePOSContext({ business_id: business, branch_id: branch, permissions: { pos: true } });
  assert.deepEqual(await readCachedProducts(business, branch), products);
  assert.deepEqual(await readSavedBasket(business, branch), [{ product_id: 'p1', quantity: 3 }]);
  assert.deepEqual((await readCachedTables(business, branch)).map(x => x.id), ['t1']);
  assert.equal((await readCachedPOSContext()).branch_id, branch);
  assert.deepEqual(await readCachedProducts(business, 'another-branch'), []);
});

test('deleting a draft removes only the selected outbox record', async () => {
  const first = draft(); const second = draft();
  await enqueueDraft(first); await enqueueDraft(second);
  await deleteDraft(first.client_reference);
  const rows = await listDrafts(business, branch);
  assert.deepEqual(rows.map(x => x.client_reference), [second.client_reference]);
});

test('storage status reports advisory usage and persistence metadata', async () => {
  const original = Object.getOwnPropertyDescriptor(globalThis, 'navigator');
  Object.defineProperty(globalThis, 'navigator', { configurable: true, value: { storage: {
    estimate: async () => ({ usage: 2048, quota: 8192 }),
    persisted: async () => false,
    persist: async () => true,
  } } });
  try {
    const { getOfflineStorageStatus, requestOfflineStoragePersistence, formatStorageBytes } = await import('../src/lib/offline-pos.ts');
    assert.deepEqual(await getOfflineStorageStatus(), { supported: true, usage: 2048, quota: 8192, persisted: false });
    assert.equal(await requestOfflineStoragePersistence(), true);
    assert.equal(formatStorageBytes(1024), '1.0 KB');
    assert.equal(formatStorageBytes(null), 'Unavailable');
  } finally {
    if (original) Object.defineProperty(globalThis, 'navigator', original);
    else delete globalThis.navigator;
  }
});

test('storage status degrades safely when browser storage APIs are unavailable', async () => {
  const original = Object.getOwnPropertyDescriptor(globalThis, 'navigator');
  Object.defineProperty(globalThis, 'navigator', { configurable: true, value: {} });
  try {
    const { getOfflineStorageStatus, requestOfflineStoragePersistence } = await import('../src/lib/offline-pos.ts');
    assert.deepEqual(await getOfflineStorageStatus(), { supported: false, usage: null, quota: null, persisted: null });
    assert.equal(await requestOfflineStoragePersistence(), null);
  } finally {
    if (original) Object.defineProperty(globalThis, 'navigator', original);
    else delete globalThis.navigator;
  }
});
