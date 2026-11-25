/** Pure policy helpers shared by the IndexedDB outbox and automated tests. */
export function isStaleSync(updatedAt: string, now = Date.now(), staleAfterMs = 120_000): boolean {
  const updated = Date.parse(updatedAt);
  return !Number.isFinite(updated) || now - updated >= staleAfterMs;
}
export function canClaimDraft(state: string, updatedAt: string, now = Date.now(), staleAfterMs = 120_000): boolean {
  if (state === "synced" || state === "blocked") return false;
  if (state !== "syncing") return state === "pending";
  return isStaleSync(updatedAt, now, staleAfterMs);
}
export function nextDraftHistory<T>(history: T[] | undefined, event: T, limit = 30): T[] {
  const previous = Array.isArray(history) ? history : [];
  const safeLimit = Number.isSafeInteger(limit) && limit > 0 ? limit : 30;
  return [...previous, event].slice(-safeLimit);
}
