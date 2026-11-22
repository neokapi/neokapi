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
 *
 * Each edit belongs to the session that made it: the user, the workspace and
 * the server (its principal). An edit is sent only under a session with the
 * same principal, and only that session lists it. While no session is set
 * (the sign-in screen) an edit is neither sent nor listed; a host may show how
 * many wait. An edit whose session is gone is kept for its user to return to,
 * for at most HELD_EDIT_TTL_MS and HELD_EDIT_LIMIT edits. The record lives in
 * memory only and ends with the app.
 */

/** Who made an edit, and where. */
export interface PendingSavesPrincipal {
  userId: string;
  workspace: string;
  server: string;
}

/** How long an edit whose session is gone is kept for its user. */
export const HELD_EDIT_TTL_MS = 8 * 60 * 60 * 1000;
/** How many edits are kept in all; the oldest of another session go first. */
export const HELD_EDIT_LIMIT = 50;

let principal: PendingSavesPrincipal | null = null;
let session: { userId: string; workspace: string } | null = null;
// The web app talks to its own origin; the desktop names its server.
let server: string | null = typeof window === "undefined" ? null : window.location.origin;

function samePrincipal(a: PendingSavesPrincipal | null, b: PendingSavesPrincipal | null) {
  return (
    a != null &&
    b != null &&
    a.userId === b.userId &&
    a.workspace === b.workspace &&
    a.server === b.server
  );
}

function updatePrincipal(): void {
  const next = session && server ? { ...session, server } : null;
  if (samePrincipal(principal, next) || (principal == null && next == null)) return;
  principal = next;
  prune();
  changed();
}

/**
 * Sets the signed-in user and the workspace they are in; null when the
 * session ends. The shared app sets it from its workspace route.
 */
export function setPendingSavesSession(next: { userId: string; workspace: string } | null): void {
  session = next;
  updatePrincipal();
}

/** Sets the server the app is connected to (the desktop names it). */
export function setPendingSavesServer(next: string | null): void {
  server = next;
  updatePrincipal();
}

/** An edit made under another session, which this session never sends. */
export class HeldEditError extends Error {
  constructor() {
    super("this edit was made in another session");
    this.name = "HeldEditError";
  }
}

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
export type FlushResult =
  | { ok: true }
  | {
      ok: false;
      timedOut: boolean;
      errors: FlushError[];
      /** Edits not sent because no session is signed in, counted only. */
      held?: number;
    };

interface Entry extends PendingEditInput {
  /** The session that made the edit; null when none was set. */
  owner: PendingSavesPrincipal | null;
  recordedAt: number;
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

/** Whether the current session may see and send e. */
function owned(e: Entry): boolean {
  return samePrincipal(e.owner, principal);
}

/** Drops edits of another session past their time, and the oldest past the limit. */
function prune(): void {
  const now = Date.now();
  for (const [key, e] of entries) {
    if (!owned(e) && !e.saving && now - e.recordedAt > HELD_EDIT_TTL_MS) entries.delete(key);
  }
  const held = [...entries.values()].filter((e) => !owned(e) && !e.saving);
  held.sort((x, y) => x.recordedAt - y.recordedAt);
  while (entries.size > HELD_EDIT_LIMIT && held.length > 0) entries.delete(held.shift()!.key);
}

function changed(): void {
  snapshot = [...entries.values()].filter(owned).map((e) => ({
    key: e.key,
    label: e.label,
    scope: e.scope,
    state: e.saving ? "saving" : e.error != null ? "failed" : "resting",
    error: e.error,
    data: e.data,
  }));
  for (const listener of listeners) listener();
}

/**
 * Records an edit under the current session, replacing any earlier unsaved
 * edit to the same key. Without a session there is no one to save it for, so
 * nothing is recorded. An earlier edit to the key from another session is
 * replaced.
 */
export function recordPendingEdit(edit: PendingEditInput): void {
  if (principal == null) return;
  const prev = entries.get(edit.key);
  if (prev && owned(prev)) {
    // The entry object stays the same, so a save under way still finds it.
    Object.assign(prev, edit, { gen: prev.gen + 1, recordedAt: Date.now() });
  } else {
    entries.set(edit.key, {
      ...edit,
      owner: principal,
      recordedAt: Date.now(),
      gen: 1,
      saving: false,
      error: null,
      chain: Promise.resolve(),
    });
  }
  prune();
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
  if (!owned(entry)) return Promise.reject(new HeldEditError());
  const run = entry.chain.then(async () => {
    if (entries.get(key) !== entry) return;
    // The session may have changed while an earlier save of the key ran.
    if (!owned(entry)) throw new HeldEditError();
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

/** Drops a recorded edit the person chose not to keep, if it is theirs. */
export function discardPendingEdit(key: string): void {
  const entry = entries.get(key);
  if (entry && owned(entry) && entries.delete(key)) changed();
}

/** Drops every recorded edit, every session's (an explicit sign-out). */
export function discardAllPendingSaves(): void {
  if (entries.size === 0) return;
  entries.clear();
  changed();
}

/** The current session's recorded edits; the same array until one changes. */
export function pendingEdits(): PendingEdit[] {
  return snapshot;
}

/**
 * Whether the current session has an edit that has not reached the server.
 * Without a session, whether any edit waits for its session to return.
 */
export function hasPendingSaves(): boolean {
  if (principal == null) return entries.size > 0;
  for (const e of entries.values()) if (owned(e)) return true;
  return false;
}

/** How many edits wait for a session to return; for a screen without one. */
export function waitingEditCount(): number {
  return principal == null ? entries.size : 0;
}

/** Calls listener whenever the recorded edits change. */
export function subscribePendingSaves(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

/**
 * Saves the current session's recorded edits at once and waits for all of
 * them, at most timeoutMs. A save that fails, or a wait that runs out, is
 * reported, and the edit stays recorded. Another session's edits are never
 * sent. Without a session nothing is sent, and the result counts the edits
 * that wait (held) without naming them.
 */
export async function flushPendingSaves(timeoutMs: number): Promise<FlushResult> {
  if (principal == null) {
    return entries.size === 0
      ? { ok: true }
      : { ok: false, timedOut: false, errors: [], held: entries.size };
  }
  const waiting = [...entries.values()].filter(owned);
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
