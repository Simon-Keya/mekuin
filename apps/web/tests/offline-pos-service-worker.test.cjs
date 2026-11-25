const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');

function createWorker() {
  const handlers = new Map();
  const store = new Map();
  const cache = {
    async match(key) { return store.get(typeof key === 'string' ? key : key.url); },
    async put(key, value) { store.set(typeof key === 'string' ? key : key.url, value); },
  };
  const cacheApi = {
    async open() { return cache; },
    async keys() { return ['meku-in-shell-v1']; },
    async delete() { return true; },
  };
  const worker = {
    skipWaiting: async () => {}, clients: { claim: async () => {} },
    location: { origin: 'https://meku.test' },
    addEventListener(name, callback) { handlers.set(name, callback); },
  };
  const context = { self: worker, caches: cacheApi, URL, Response, Request, fetch: async () => new Response('network') };
  vm.runInNewContext(fs.readFileSync(require.resolve('../public/sw.js'), 'utf8'), context);
  return { handlers, store };
}

test('service worker installs and activates without failing', async () => {
  const { handlers } = createWorker();
  handlers.get('install')({ waitUntil(p) { this.promise = p; } });
  handlers.get('activate')({ waitUntil(p) { this.promise = p; } });
});

test('API, auth and tenant requests are never intercepted for caching', () => {
  const { handlers } = createWorker();
  for (const path of ['/api/v1/sales/orders', '/auth/session', '/tenant/context']) {
    let intercepted = false;
    handlers.get('fetch')({ request: new Request(`https://meku.test${path}`), respondWith() { intercepted = true; } });
    assert.equal(intercepted, false, `${path} must bypass the service worker cache`);
  }
});

test('non-GET and cross-origin requests bypass the service worker', () => {
  const { handlers } = createWorker();
  for (const request of [
    new Request('https://meku.test/pos', { method: 'POST' }),
    new Request('https://third-party.test/script.js'),
  ]) {
    let intercepted = false;
    handlers.get('fetch')({ request, respondWith() { intercepted = true; } });
    assert.equal(intercepted, false);
  }
});

test('static asset requests use network-first cache population', async () => {
  const { handlers, store } = createWorker();
  let responsePromise;
  handlers.get('fetch')({
    request: new Request('https://meku.test/_next/static/chunk.js'),
    respondWith(p) { responsePromise = p; },
  });
  const response = await responsePromise;
  assert.equal(await response.text(), 'network');
  assert.equal(store.has('https://meku.test/_next/static/chunk.js'), true);
});

test('offline POS navigation has a deterministic uncached fallback', async () => {
  let responsePromise;
  {
    const contextFetch = async () => { throw new Error('offline'); };
    // Rebuild with an offline network implementation.
    const handlersOffline = new Map();
    const worker = { skipWaiting: async () => {}, clients: { claim: async () => {} }, location: { origin: 'https://meku.test' }, addEventListener: (n, cb) => handlersOffline.set(n, cb) };
    const cache = { match: async () => undefined, put: async () => {} };
    vm.runInNewContext(fs.readFileSync(require.resolve('../public/sw.js'), 'utf8'), {
      self: worker, caches: { open: async () => cache, keys: async () => [], delete: async () => true },
      URL, Response, Request, fetch: contextFetch,
    });
    handlersOffline.get('fetch')({ request: { method: 'GET', url: 'https://meku.test/pos', mode: 'navigate' }, respondWith(p) { responsePromise = p; } });
    const response = await responsePromise;
    assert.equal(response.status, 503);
    assert.match(await response.text(), /Offline page not yet cached/);
  }
});
