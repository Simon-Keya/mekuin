const test = require('node:test');
const assert = require('node:assert/strict');
const policy = require('../src/lib/offline-pos-policy.mjs');

const NOW = Date.parse('2026-11-25T12:00:00.000Z');
test('pending drafts can be claimed', () => assert.equal(policy.canClaimDraft('pending', '', NOW), true));
test('synced and blocked drafts cannot be claimed', () => {
  assert.equal(policy.canClaimDraft('synced', '', NOW), false);
  assert.equal(policy.canClaimDraft('blocked', '', NOW), false);
});
test('recent syncing draft is protected from another tab', () =>
  assert.equal(policy.canClaimDraft('syncing', new Date(NOW - 119_999).toISOString(), NOW), false));
test('syncing draft at stale threshold can be reclaimed', () =>
  assert.equal(policy.canClaimDraft('syncing', new Date(NOW - 120_000).toISOString(), NOW), true));
test('invalid syncing timestamp is recoverable', () =>
  assert.equal(policy.canClaimDraft('syncing', 'not-a-date', NOW), true));
test('future timestamp is treated as recent, not stale', () =>
  assert.equal(policy.isStaleSync(new Date(NOW + 60_000).toISOString(), NOW), false));
test('history is bounded to the newest events', () => {
  const history = Array.from({ length: 30 }, (_, i) => i);
  assert.deepEqual(policy.nextDraftHistory(history, 30), Array.from({ length: 30 }, (_, i) => i + 1));
});
test('malformed history and invalid limits are handled safely', () => {
  assert.deepEqual(policy.nextDraftHistory(undefined, 'event', 0), ['event']);
  assert.deepEqual(policy.nextDraftHistory('bad', 'event'), ['event']);
});
