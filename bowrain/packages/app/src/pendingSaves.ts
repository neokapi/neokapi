/**
 * Edits that have not reached the server, one record per edited thing (one
 * flow, say), kept outside the component that made them so a surface that
 * closes and opens again still has them.
 *
 * Here it holds the edits a save was refused for because someone else saved
 * the same thing first: the person's version stays recorded, the surface
 * shows that it is not saved, and only the person's explicit choice (keep the
 * current version, or save theirs over it) clears it. The record lives in
 * memory only.
 */

/** One edit as a surface records it. */
export interface PendingEditInput {
  /** What the edit is to; one record per key. */
  key: string;
  /** The name a person knows the edited thing by. */
  label: string;
  /** Groups the keys one surface lists (a project's flows, say). */
  scope: string;
  /** Why the edit is not saved, when a save was refused. */
  error?: unknown;
  /** The edit itself, so a surface that opens again can show it. */
  data?: unknown;
}

/** Where one recorded edit stands. */
export type PendingEditState = "resting" | "failed";

/** A recorded edit, as a surface reads it. */
export interface PendingEdit {
  key: string;
  label: string;
  scope: string;
  state: PendingEditState;
  error: unknown;
  data?: unknown;
}

const entries = new Map<string, PendingEdit>();
const listeners = new Set<() => void>();
let snapshot: PendingEdit[] = [];

function changed(): void {
  snapshot = [...entries.values()];
  for (const listener of listeners) listener();
}

/** Records an edit, replacing any earlier record of the same key. */
export function recordPendingEdit(edit: PendingEditInput): void {
  entries.set(edit.key, {
    key: edit.key,
    label: edit.label,
    scope: edit.scope,
    state: edit.error != null ? "failed" : "resting",
    error: edit.error ?? null,
    data: edit.data,
  });
  changed();
}

/** The record for key, if any. */
export function pendingEdit(key: string): PendingEdit | undefined {
  return entries.get(key);
}

/** Drops a record the person resolved. */
export function discardPendingEdit(key: string): void {
  if (entries.delete(key)) changed();
}

/** Drops every record. For tests. */
export function discardAllPendingSaves(): void {
  if (entries.size === 0) return;
  entries.clear();
  changed();
}

/** Every record; the same array until one changes. */
export function pendingEdits(): PendingEdit[] {
  return snapshot;
}

/** Calls listener whenever the records change. */
export function subscribePendingSaves(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}
