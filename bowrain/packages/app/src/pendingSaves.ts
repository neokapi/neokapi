/**
 * Saves that are waiting to happen: an edit resting before its delayed save,
 * or a save still on its way to the server.
 *
 * A surface that delays or queues a save registers here, so a host that is
 * about to go away (the desktop window closing, the app quitting) can flush
 * every one of them first and learn whether they all reached the server.
 */

/** One surface's waiting saves. */
export interface PendingSave {
  /** Whether an edit is resting or a save is in flight. */
  pending: () => boolean;
  /** Saves now; resolves once saved, rejects with the save's error. */
  flush: () => Promise<void>;
}

/** The outcome of flushing every waiting save. */
export type FlushResult = { ok: true } | { ok: false; timedOut: boolean; errors: unknown[] };

const entries = new Set<PendingSave>();
const listeners = new Set<() => void>();

/** Registers a surface's waiting saves; returns the unregister call. */
export function registerPendingSave(entry: PendingSave): () => void {
  entries.add(entry);
  notifyPendingSaves();
  return () => {
    entries.delete(entry);
    notifyPendingSaves();
  };
}

/** Whether any registered surface has a save waiting. */
export function hasPendingSaves(): boolean {
  for (const entry of entries) if (entry.pending()) return true;
  return false;
}

/** Tells subscribers that a surface's waiting saves changed. */
export function notifyPendingSaves(): void {
  for (const listener of listeners) listener();
}

/** Calls listener whenever the waiting saves may have changed. */
export function subscribePendingSaves(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

/**
 * Flushes every waiting save at once and waits for all of them, at most
 * timeoutMs. A save that fails, or a wait that runs out, is reported rather
 * than dropped.
 */
export async function flushPendingSaves(timeoutMs: number): Promise<FlushResult> {
  const waiting = [...entries].filter((entry) => entry.pending());
  if (waiting.length === 0) return { ok: true };
  let timer: ReturnType<typeof setTimeout> | undefined;
  const timeout = new Promise<"timeout">((resolve) => {
    timer = setTimeout(() => resolve("timeout"), timeoutMs);
  });
  const all = Promise.allSettled(waiting.map((entry) => entry.flush()));
  const outcome = await Promise.race([all, timeout]);
  clearTimeout(timer);
  if (outcome === "timeout") return { ok: false, timedOut: true, errors: [] };
  const errors = outcome
    .filter((r): r is PromiseRejectedResult => r.status === "rejected")
    .map((r) => r.reason);
  return errors.length === 0 ? { ok: true } : { ok: false, timedOut: false, errors };
}
