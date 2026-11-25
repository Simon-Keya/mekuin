export function isStaleSync(updatedAt: string, now?: number, staleAfterMs?: number): boolean;
export function canClaimDraft(state: string, updatedAt: string, now?: number, staleAfterMs?: number): boolean;
export function nextDraftHistory<T>(history: T[] | undefined, event: T, limit?: number): T[];
