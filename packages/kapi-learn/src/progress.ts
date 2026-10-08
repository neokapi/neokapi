// Where a reader got to in each lab, kept in this browser.
//
// A per-viewer convenience, like a video site's "continue watching": the
// index shows a progress bar on each card and the player resumes at the last
// chapter. Nothing here is shared or read back; it lives in localStorage and
// every read is guarded, since storage can be unavailable or cleared.

export interface LabProgress {
  /** Chapter ids the reader has run, in order. */
  done: string[];
  /** The chapter the reader was on. */
  at?: string;
  /** When the lab was last opened, as epoch milliseconds. */
  updated: number;
}

const KEY = "kapi-learn:progress";

type Store = Record<string, LabProgress>;

function read(): Store {
  try {
    const raw = globalThis.localStorage?.getItem(KEY);
    if (!raw) return {};
    const parsed = JSON.parse(raw) as unknown;
    return parsed && typeof parsed === "object" ? (parsed as Store) : {};
  } catch {
    return {};
  }
}

function write(store: Store): void {
  try {
    globalThis.localStorage?.setItem(KEY, JSON.stringify(store));
  } catch {
    /* storage unavailable: progress is a convenience */
  }
}

export function getProgress(labId: string): LabProgress | undefined {
  return read()[labId];
}

export function allProgress(): Store {
  return read();
}

/** Record that a chapter ran, and that the reader is on it. */
export function markChapter(labId: string, chapterId: string): void {
  const store = read();
  const current = store[labId] ?? { done: [], updated: 0 };
  const done = current.done.includes(chapterId) ? current.done : [...current.done, chapterId];
  store[labId] = { done, at: chapterId, updated: Date.now() };
  write(store);
}

/** Record where the reader is without marking the chapter done. */
export function markPosition(labId: string, chapterId: string): void {
  const store = read();
  const current = store[labId] ?? { done: [], updated: 0 };
  store[labId] = { ...current, at: chapterId, updated: Date.now() };
  write(store);
}

export function clearProgress(labId: string): void {
  const store = read();
  delete store[labId];
  write(store);
}

/** The most recently opened lab, for the index's "continue" card. */
export function lastOpened(): { labId: string; progress: LabProgress } | undefined {
  const store = read();
  let best: { labId: string; progress: LabProgress } | undefined;
  for (const [labId, progress] of Object.entries(store)) {
    if (!best || progress.updated > best.progress.updated) best = { labId, progress };
  }
  return best;
}
