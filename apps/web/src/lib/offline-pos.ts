import { canClaimDraft, isStaleSync, nextDraftHistory } from "./offline-pos-policy.mjs";
export type OfflineProduct = { id: string; sku: string; name: string; sale_price_minor: number; is_active: boolean; is_sellable: boolean };
export type OfflineLine = { product_id: string; quantity: number };
export type OfflineTable = { id: string; table_code: string; display_name: string; capacity: number; area?: string | null; is_active: boolean };
export type DraftSyncEvent = { at: string; state: OutboxDraft["state"]; message: string; attempt: number };
export type OutboxDraft = {
  client_reference: string;
  business_id: string;
  branch_id: string;
  service_type: "walk_in" | "table" | "tab";
  table_id: string;
  label: string;
  lines: OfflineLine[];
  state: "pending" | "syncing" | "synced" | "blocked";
  attempts: number;
  last_error: string;
  server_order_id: string;
  created_at: string;
  updated_at: string;
  history?: DraftSyncEvent[];
};

type StoreName = "products" | "outbox" | "context" | "basket" | "tables";
const DB_NAME = "meku-in-offline-pos";
const DB_VERSION = 2;

function openDB(): Promise<IDBDatabase> {
  if (typeof indexedDB === "undefined") return Promise.reject(new Error("Offline storage is unavailable in this browser."));
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(DB_NAME, DB_VERSION);
    request.onupgradeneeded = () => {
      const db = request.result;
      if (!db.objectStoreNames.contains("products")) db.createObjectStore("products", { keyPath: "key" });
      if (!db.objectStoreNames.contains("outbox")) db.createObjectStore("outbox", { keyPath: "client_reference" });
      if (!db.objectStoreNames.contains("context")) db.createObjectStore("context", { keyPath: "key" });
      if (!db.objectStoreNames.contains("basket")) db.createObjectStore("basket", { keyPath: "key" });
      if (!db.objectStoreNames.contains("tables")) db.createObjectStore("tables", { keyPath: "key" });
    };
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error ?? new Error("Could not open offline storage."));
  });
}

async function request<T>(store: StoreName, mode: IDBTransactionMode, run: (objectStore: IDBObjectStore) => IDBRequest<T>): Promise<T> {
  const db = await openDB();
  try {
    return await new Promise<T>((resolve, reject) => {
      const tx = db.transaction(store, mode);
      const req = run(tx.objectStore(store));
      let result: T | undefined;
      req.onsuccess = () => { result = req.result; };
      req.onerror = () => reject(req.error ?? new Error("Offline storage request failed."));
      tx.oncomplete = () => resolve(result as T);
      tx.onerror = () => reject(tx.error ?? new Error("Offline storage transaction failed."));
      tx.onabort = () => reject(tx.error ?? new Error("Offline storage transaction failed."));
    });
  } finally { db.close(); }
}


export type OfflineStorageStatus = {
  supported: boolean;
  usage: number | null;
  quota: number | null;
  persisted: boolean | null;
};

/** Read browser-managed storage information without changing or evicting data. */
export async function getOfflineStorageStatus(): Promise<OfflineStorageStatus> {
  if (typeof navigator === "undefined" || !navigator.storage) {
    return { supported: false, usage: null, quota: null, persisted: null };
  }
  let usage: number | null = null;
  let quota: number | null = null;
  let persisted: boolean | null = null;
  try {
    const estimate = await navigator.storage.estimate();
    usage = typeof estimate.usage === "number" ? estimate.usage : null;
    quota = typeof estimate.quota === "number" ? estimate.quota : null;
  } catch { /* Storage estimates are advisory and may be unavailable. */ }
  try {
    if (typeof navigator.storage.persisted === "function") persisted = await navigator.storage.persisted();
  } catch { /* Persistence status is optional browser metadata. */ }
  return { supported: true, usage, quota, persisted };
}

/** Must be called from an explicit operator action; browsers may decline. */
export async function requestOfflineStoragePersistence(): Promise<boolean | null> {
  if (typeof navigator === "undefined" || !navigator.storage || typeof navigator.storage.persist !== "function") return null;
  return navigator.storage.persist();
}

export function formatStorageBytes(bytes: number | null): string {
  if (bytes === null || !Number.isFinite(bytes) || bytes < 0) return "Unavailable";
  if (bytes < 1024) return `${Math.round(bytes)} B`;
  const units = ["KB", "MB", "GB"];
  let value = bytes / 1024;
  let index = 0;
  while (value >= 1024 && index < units.length - 1) { value /= 1024; index += 1; }
  return `${value.toFixed(1)} ${units[index]}`;
}

const scope = (businessId: string, branchId: string) => `${businessId}:${branchId}`;
export async function cachePOSContext(context: { business_id: string; branch_id: string; permissions: Record<string, boolean> }) {
  await request("context", "readwrite", store => store.put({ key: "active", context, cached_at: new Date().toISOString() }));
}
export async function readCachedPOSContext(): Promise<{ business_id: string; branch_id: string; permissions: Record<string, boolean> } | null> {
  const value = await request<{ context: { business_id: string; branch_id: string; permissions: Record<string, boolean> } } | undefined>("context", "readonly", store => store.get("active"));
  return value?.context ?? null;
}
export async function cacheProducts(businessId: string, branchId: string, products: OfflineProduct[]) {
  await request("products", "readwrite", store => store.put({ key: scope(businessId, branchId), products, cached_at: new Date().toISOString() }));
}
export async function readCachedProducts(businessId: string, branchId: string): Promise<OfflineProduct[]> {
  const value = await request<{ products: OfflineProduct[] } | undefined>("products", "readonly", store => store.get(scope(businessId, branchId)));
  return value?.products ?? [];
}
export async function saveBasket(businessId: string, branchId: string, lines: OfflineLine[]) {
  await request("basket", "readwrite", store => store.put({ key: scope(businessId, branchId), lines, updated_at: new Date().toISOString() }));
}
export async function readSavedBasket(businessId: string, branchId: string): Promise<OfflineLine[]> {
  const value = await request<{ lines: OfflineLine[] } | undefined>("basket", "readonly", store => store.get(scope(businessId, branchId)));
  return value?.lines ?? [];
}
export async function enqueueDraft(draft: OutboxDraft) {
  const initial = draft.history?.length ? draft.history : [{ at: draft.created_at, state: draft.state, message: "Draft saved on this device.", attempt: draft.attempts }];
  await request("outbox", "readwrite", store => store.put({ ...draft, history: initial.slice(-30) }));
}
export async function updateDraft(draft: OutboxDraft) {
  // Read, derive the transition history, and write inside one readwrite transaction.
  // Separate readonly/readwrite transactions can lose events when two tabs update
  // the same draft concurrently (for example, sync completion racing diagnostics).
  const db = await openDB();
  try {
    await new Promise<void>((resolve, reject) => {
      const tx = db.transaction("outbox", "readwrite");
      const store = tx.objectStore("outbox");
      const get = store.get(draft.client_reference);
      get.onsuccess = () => {
        const previous = get.result as OutboxDraft | undefined;
        const changed = !previous || previous.state !== draft.state || previous.last_error !== draft.last_error || previous.server_order_id !== draft.server_order_id || previous.attempts !== draft.attempts;
        const message = draft.state === "synced" ? (draft.last_error || "Server order confirmed.") : draft.state === "blocked" ? (draft.last_error || "Draft requires operator attention.") : draft.state === "syncing" ? "Sending draft to the server." : (draft.last_error || (draft.attempts > (previous?.attempts ?? 0) ? "Sync attempt did not complete; draft remains queued." : "Draft updated."));
        const existingHistory = Array.isArray(previous?.history) ? previous.history : Array.isArray(draft.history) ? draft.history : [];
        const history = changed ? nextDraftHistory(existingHistory, { at: draft.updated_at, state: draft.state, message, attempt: draft.attempts }) : existingHistory.slice(-30);
        store.put({ ...draft, history });
      };
      get.onerror = () => reject(get.error ?? new Error("Could not read draft history."));
      tx.oncomplete = () => resolve();
      tx.onerror = () => reject(tx.error ?? new Error("Could not update draft history."));
      tx.onabort = () => reject(tx.error ?? new Error("Draft history update was interrupted."));
    });
  } finally { db.close(); }
}

/** Atomically claim a queued draft before network I/O. This is the final guard
 * against two tabs replaying the same record when Web Locks is unavailable. */
export async function claimDraft(clientReference: string, staleAfterMs = 120_000): Promise<OutboxDraft | null> {
  const db = await openDB();
  try {
    return await new Promise<OutboxDraft | null>((resolve, reject) => {
      const tx = db.transaction("outbox", "readwrite");
      const store = tx.objectStore("outbox");
      let claimed: OutboxDraft | null = null;
      const get = store.get(clientReference);
      get.onsuccess = () => {
        const current = get.result as OutboxDraft | undefined;
        if (!current || !canClaimDraft(current.state, current.updated_at, Date.now(), staleAfterMs)) return;
        const now = new Date().toISOString();
        claimed = { ...current, state: "syncing", attempts: current.attempts + 1, updated_at: now };
        const history = nextDraftHistory(Array.isArray(current.history) ? current.history : [], { at: now, state: "syncing" as const, message: "Draft claimed by this tab for server sync.", attempt: claimed.attempts });
        claimed.history = history;
        store.put(claimed);
      };
      get.onerror = () => reject(get.error ?? new Error("Could not claim queued draft."));
      tx.oncomplete = () => resolve(claimed);
      tx.onerror = () => reject(tx.error ?? new Error("Draft claim transaction failed."));
      tx.onabort = () => reject(tx.error ?? new Error("Draft claim transaction was interrupted."));
    });
  } finally { db.close(); }
}

export async function listDrafts(businessId: string, branchId: string): Promise<OutboxDraft[]> {
  const all = await request<OutboxDraft[]>("outbox", "readonly", store => store.getAll());
  return all.filter(item => item.business_id === businessId && item.branch_id === branchId).sort((a,b) => a.created_at.localeCompare(b.created_at));
}

export async function deleteDraft(clientReference: string) {
  await request("outbox", "readwrite", store => store.delete(clientReference));
}

export async function cacheTables(businessId: string, branchId: string, tables: OfflineTable[]) {
  await request("tables", "readwrite", store => store.put({ key: scope(businessId, branchId), tables, cached_at: new Date().toISOString() }));
}
export async function readCachedTables(businessId: string, branchId: string): Promise<OfflineTable[]> {
  const value = await request<{ tables: OfflineTable[] } | undefined>("tables", "readonly", store => store.get(scope(businessId, branchId)));
  return (value?.tables ?? []).filter(table => table.is_active);
}

/** Recover drafts left in `syncing` by a tab/browser crash. Replaying is safe because
 * the server's client_reference idempotency key is stable across retries. */
export async function recoverInterruptedDrafts(businessId: string, branchId: string, staleAfterMs = 120_000): Promise<number> {
  // A `syncing` state can belong to another open tab. Only recover entries whose
  // heartbeat is old enough to indicate a crashed/abandoned sync, never a live one.
  const drafts = await listDrafts(businessId, branchId);
  const now = Date.now();
  const interrupted = drafts.filter(item => {
    if (item.state !== "syncing") return false;
    return isStaleSync(item.updated_at, now, staleAfterMs);
  });
  for (const item of interrupted) {
    await updateDraft({
      ...item,
      state: "pending",
      last_error: "Recovered after an interrupted sync. The same reference will be retried safely.",
      updated_at: new Date().toISOString(),
    });
  }
  return interrupted.length;
}
