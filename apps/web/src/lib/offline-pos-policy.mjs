/** Pure policy helpers shared by the IndexedDB outbox and automated tests. */
export function isStaleSync(updatedAt, now = Date.now(), staleAfterMs = 120_000) {
  const updated = Date.parse(updatedAt);
  return !Number.isFinite(updated) || now - updated >= staleAfterMs;
}

export function canClaimDraft(state, updatedAt, now = Date.now(), staleAfterMs = 120_000) {
  if (state === "synced" || state === "blocked") return false;
  if (state !== "syncing") return state === "pending";
  return isStaleSync(updatedAt, now, staleAfterMs);
}

export function nextDraftHistory(history, event, limit = 30) {
  const previous = Array.isArray(history) ? history : [];
  const safeLimit = Number.isSafeInteger(limit) && limit > 0 ? limit : 30;
  return [...previous, event].slice(-safeLimit);
}
