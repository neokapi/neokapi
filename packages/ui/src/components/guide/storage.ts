/**
 * The ids of finished guides, kept in localStorage under one key as a JSON
 * array. Every access is guarded: with storage blocked or missing the set
 * starts empty and lives only in memory.
 */
export function readCompleted(storageKey: string): Set<string> {
  try {
    const raw = globalThis.localStorage?.getItem(storageKey);
    if (!raw) return new Set();
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) return new Set();
    return new Set(parsed.filter((id): id is string => typeof id === "string"));
  } catch {
    return new Set();
  }
}

export function writeCompleted(storageKey: string, completed: ReadonlySet<string>): void {
  try {
    globalThis.localStorage?.setItem(storageKey, JSON.stringify([...completed]));
  } catch {
    // Storage is unavailable; the set stays in memory for this session.
  }
}
