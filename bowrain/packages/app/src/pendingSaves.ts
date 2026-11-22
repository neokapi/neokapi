/**
 * Edits that have not reached the server yet.
 *
 * A surface that delays a save records its edit here, keyed by what it edits
 * (one flow, say). The record lives in this module rather than in the
 * component that made the edit, so it survives the component unmounting, a
 * route change and a lost session. An edit stays recorded until a save of
 * that same key succeeds or the person discards it. A failed save keeps it,
 * and one key's success never clears another key.
 *
 * A host that is about to go away (the desktop window closing, the app
 * quitting) flushes every recorded edit and learns which ones the server did
 * not take.
 */

/** One edit as a surface records it. */
export interface PendingEditInput {
  /** What the edit is to; one record per key. */
  key: string;
  /** The name a person knows the edited thing by. */
  label: string;
  /** Groups the keys one surface lists (a project's flows, say). */
  scope: string;
  /** Sends this edit to the server. */
  send: () => Promise<void>;
  /** The edit itself, so a surface that mounts again can show it. */
  data?: unknown;
}

/** Where one recorded edit stands. */
export type PendingEditState = "resting" | "saving" | "failed";

/** A recorded edit, as a surface reads it. */
export interface PendingEdit {
  key: string;
  label: string;
  scope: string;
  state: PendingEditState;
  error: unknown;
  data?: unknown;
}

/** One edit the server did not take. */
export interface FlushError {
  key: string;
  label: string;
  error: unknown;
}

/** The outcome of flushing every recorded edit. */
export type FlushResult = { ok: true } | { ok: false; timedOut: boolean; errors: FlushError[] };

interface Entry extends PendingEditInput {
  /** Bumped on every edit, so a save knows whether a newer edit came after it. */
  gen: number;
  saving: boolean;
  error: unknown;
  /** Saves of this key run one after another. */
  chain: Promise<void>;
}

const entries = new Map<string, Entry>();
const listeners = new Set<() => void>();
let snapshot: PendingEdit[] = [];

function changed(): void {
  snapshot = [...entries.values()].map((e) => ({
    key: e.key,
    label: e.label,
    scope: e.scope,
    state: e.saving ? "saving" : e.error != null ? "failed" : "resting",
    error: e.error,
    data: e.data,
  }));
  for (const listener of listeners) listener();
}

/** Records an edit, replacing any earlier unsaved edit to the same key. */
export function recordPendingEdit(edit: PendingEditInput): void {
  const prev = entries.get(edit.key);
  if (prev) {
    // The entry object stays the same, so a save under way still finds it.
    Object.assign(prev, edit, { gen: prev.gen + 1 });
  } else {
    entries.set(edit.key, {
      ...edit,
      gen: 1,
      saving: false,
      error: null,
      chain: Promise.resolve(),
    });
  }
  changed();
}

/**
 * Saves the key's recorded edit, after any save of the key already under way.
 * Resolves when the server took it (or nothing was recorded) and rejects with
 * the save's error, which stays on the record.
 */
export function savePendingEdit(key: string): Promise<void> {
  const entry = entries.get(key);
  if (!entry) return Promise.resolve();
  const run = entry.chain.then(async () => {
    if (entries.get(key) !== entry) return;
    const gen = entry.gen;
    entry.saving = true;
    changed();
    try {
      await entry.send();
      entry.error = null;
      if (entries.get(key) === entry && entry.gen === gen) entries.delete(key);
    } catch (e) {
      entry.error = e;
      throw e;
    } finally {
      entry.saving = false;
      changed();
    }
  });
  // The next save waits for this one whatever its outcome.
  entry.chain = run.catch(() => undefined);
  return run;
}

/** Drops a recorded edit the person chose not to keep. */
export function discardPendingEdit(key: string): void {
  if (entries.delete(key)) changed();
}

/** Drops every recorded edit. */
export function discardAllPendingSaves(): void {
  if (entries.size === 0) return;
  entries.clear();
  changed();
}

/** The recorded edits; the same array until one changes. */
export function pendingEdits(): PendingEdit[] {
  return snapshot;
}

/** Whether any edit has not reached the server. */
export function hasPendingSaves(): boolean {
  return entries.size > 0;
}

/** Calls listener whenever the recorded edits change. */
export function subscribePendingSaves(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

/**
 * Saves every recorded edit at once and waits for all of them, at most
 * timeoutMs. A save that fails, or a wait that runs out, is reported, and the
 * edit stays recorded.
 */
export async function flushPendingSaves(timeoutMs: number): Promise<FlushResult> {
  const waiting = [...entries.values()];
  if (waiting.length === 0) return { ok: true };
  let timer: ReturnType<typeof setTimeout> | undefined;
  const timeout = new Promise<"timeout">((resolve) => {
    timer = setTimeout(() => resolve("timeout"), timeoutMs);
  });
  const all = Promise.allSettled(waiting.map((e) => savePendingEdit(e.key)));
  const outcome = await Promise.race([all, timeout]);
  clearTimeout(timer);
  if (outcome === "timeout") return { ok: false, timedOut: true, errors: [] };
  const errors: FlushError[] = [];
  outcome.forEach((r, i) => {
    if (r.status === "rejected") {
      errors.push({ key: waiting[i].key, label: waiting[i].label, error: r.reason });
    }
  });
  return errors.length === 0 ? { ok: true } : { ok: false, timedOut: false, errors };
}
