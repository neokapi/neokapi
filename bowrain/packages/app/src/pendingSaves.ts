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
 * Each edit belongs to the person who made it on the server they made it on
 * (its principal), captured when the edit is recorded. Where an edit goes (its
 * workspace and project) is part of the edit, so it is sent there whichever
 * workspace is on screen. An edit is sent, listed and counted only while the
 * same user is signed in to the same server. While no one is signed in (the
 * sign-in screen) nothing is sent and a host may show how many edits wait,
 * never which. Another user signed in sees nothing of them.
 *
 * Nothing is dropped behind the person's back: an edit leaves the record when
 * a save of it succeeds, when its user discards it, or when its user signs out
 * after being asked. The record lives in memory only and ends with the app.
 */

/** Who made an edit, and on which server. */
export interface PendingSavesPrincipal {
  userId: string;
  server: string;
}

let principal: PendingSavesPrincipal | null = null;
let userId: string | null = null;
// The web app talks to its own origin; the desktop names its server.
let server: string | null =
  typeof window === "undefined" ? null : normalizeServer(window.location.origin);

/**
 * The server's address as edits compare it: scheme and host in lower case,
 * no trailing slash. An address that does not parse is kept as given.
 */
export function normalizeServer(raw: string): string {
  try {
    const u = new URL(raw);
    return `${u.protocol}//${u.host}${u.pathname.replace(/\/+$/, "")}`;
  } catch {
    return raw.replace(/\/+$/, "");
  }
}

function samePrincipal(a: PendingSavesPrincipal | null, b: PendingSavesPrincipal | null) {
  return a != null && b != null && a.userId === b.userId && a.server === b.server;
}

function updatePrincipal(): void {
  const next = userId && server ? { userId, server } : null;
  if (samePrincipal(principal, next) || (principal == null && next == null)) return;
  principal = next;
  changed();
}

/** Sets the signed-in user; null when the session ends. The shared app sets it. */
export function setPendingSavesUser(next: string | null): void {
  userId = next || null;
  updatePrincipal();
}

/** Sets the server the app is connected to (the desktop names it); null when none. */
export function setPendingSavesServer(next: string | null): void {
  server = next ? normalizeServer(next) : null;
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
  /** The person and server the edit was made under. */
  owner: PendingSavesPrincipal;
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
 * Records an edit under the person signed in now, replacing any earlier
 * unsaved edit of theirs to the same key. Without anyone signed in there is no
 * one to save it for, so nothing is recorded. Another user's edit to the same
 * key is kept under its own key and never replaced.
 */
export function recordPendingEdit(edit: PendingEditInput): void {
  if (principal == null) return;
  const owner = principal;
  const key = ownedKey(owner, edit.key);
  const prev = entries.get(key);
  if (prev) {
    // The entry object stays the same, so a save under way still finds it.
    Object.assign(prev, edit, { key: edit.key, gen: prev.gen + 1 });
  } else {
    entries.set(key, {
      ...edit,
      owner,
      gen: 1,
      saving: false,
      error: null,
      chain: Promise.resolve(),
    });
  }
  changed();
}

/** The record's key for owner's edit to key, so two users never share one. */
function ownedKey(owner: PendingSavesPrincipal, key: string): string {
  return `${owner.server}\u0000${owner.userId}\u0000${key}`;
}

/** The current user's entry for key, if any. */
function ownEntry(key: string): Entry | undefined {
  return principal ? entries.get(ownedKey(principal, key)) : undefined;
}

/**
 * Saves the key's recorded edit, after any save of the key already under way.
 * Resolves when the server took it (or nothing was recorded) and rejects with
 * the save's error, which stays on the record.
 */
export function savePendingEdit(key: string): Promise<void> {
  const entry = ownEntry(key);
  if (!entry) return Promise.resolve();
  const recordKey = ownedKey(entry.owner, key);
  const run = entry.chain.then(async () => {
    if (entries.get(recordKey) !== entry) return;
    // A different user or server since the save was asked for: the edit is
    // held for its own user and not sent under someone else.
    if (!owned(entry)) throw new HeldEditError();
    const gen = entry.gen;
    entry.saving = true;
    changed();
    try {
      await entry.send();
      entry.error = null;
      if (entries.get(recordKey) === entry && entry.gen === gen) entries.delete(recordKey);
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

/**
 * Replaces the data of the current user's record for key without counting it
 * as a new edit: a save under way that started before still clears the record
 * when it succeeds. A surface uses it to move a recorded edit onto what its
 * last save wrote.
 */
export function updatePendingEditData(key: string, update: (data: unknown) => unknown): void {
  const entry = ownEntry(key);
  if (!entry) return;
  entry.data = update(entry.data);
  changed();
}

/** Drops a recorded edit the person chose not to keep, if it is theirs. */
export function discardPendingEdit(key: string): void {
  if (principal && entries.delete(ownedKey(principal, key))) changed();
}

/**
 * Drops every edit of the person signed in now, in every workspace: an
 * explicit sign-out, after they were asked. Other users' edits stay.
 */
export function discardOwnPendingSaves(): void {
  let dropped = false;
  for (const [key, e] of entries) {
    if (owned(e)) {
      entries.delete(key);
      dropped = true;
    }
  }
  if (dropped) changed();
}

/** Drops every recorded edit of every user. For tests. */
export function discardAllPendingSaves(): void {
  if (entries.size === 0) return;
  entries.clear();
  changed();
}

/**
 * The signed-in user's recorded edits on this server, in every workspace; the
 * same array until one changes.
 */
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
 * Saves the signed-in user's recorded edits at once, each to the workspace it
 * was made in, and waits for all of them, at most timeoutMs. A save that
 * fails, or a wait that runs out, is reported, and the edit stays recorded.
 * Another user's edits are never sent. Without anyone signed in nothing is
 * sent, and the result counts the edits that wait (held) without naming them.
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
