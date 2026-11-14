/**
 * Reading a project's feed: which relayed events mean a new entry, the day an
 * entry falls on in the reader's time zone, and the one count a row leads
 * with. Labels and units belong to each client's catalog; nothing here
 * produces a sentence.
 */

import type { FeedEntry } from "./types.ts";

/**
 * The event types the server files feed entries from. A change event of one of
 * these types on the workspace event stream (`GET /:ws/events?project=`) means
 * the feed has a new or updated entry.
 */
export const FEED_SOURCE_EVENTS: readonly string[] = [
  "connector.push.completed",
  "convergence.run.completed",
  "review.decided",
  "review.bulk_approved",
  "recipe.changed",
  "context.recorded",
  "voice.profile.updated",
  "voice.rule_promoted",
  "voice.rule_auto_promoted",
  "voice.rule_rejected",
  "brand.rule.demoted",
];

/** Whether a relayed event's type files or updates a feed entry. */
export function isFeedSourceEvent(type: string): boolean {
  return FEED_SOURCE_EVENTS.includes(type) || type.startsWith("changeset.");
}

/** The entries of one day in the reader's time zone, newest first. */
export interface FeedDay {
  /** The local date, as YYYY-MM-DD. */
  key: string;
  /** Days before today: 0 is today, 1 yesterday. */
  daysAgo: number;
  /** Midnight of the day, local time. */
  date: Date;
  entries: FeedEntry[];
}

function localKey(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

function midnight(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate());
}

/**
 * Group entries by their local day, keeping the server's order. Entries with
 * an unreadable time are left out.
 */
export function groupFeedByDay(entries: readonly FeedEntry[], now: Date = new Date()): FeedDay[] {
  const today = midnight(now).getTime();
  const days: FeedDay[] = [];
  for (const entry of entries) {
    const at = new Date(entry.at);
    if (Number.isNaN(at.getTime())) continue;
    const key = localKey(at);
    let day: FeedDay | undefined = days[days.length - 1];
    if (!day || day.key !== key) {
      day = days.find((d) => d.key === key);
      if (!day) {
        const date = midnight(at);
        day = {
          key,
          date,
          daysAgo: Math.round((today - date.getTime()) / 86_400_000),
          entries: [],
        };
        days.push(day);
      }
    }
    day.entries.push(entry);
  }
  return days;
}

/** The context operations a context change counts beside its item kinds. */
const CONTEXT_OPERATIONS = ["add", "edit", "retire", "keep", "drop"];

/** The review decisions a review summary counts. */
const REVIEW_COUNTS = ["approved", "rejected", "unreviewed", "fixed", "bulk_approved"];

/** One count and what it counts, such as `{ key: "blocks", n: 40 }`. */
export interface FeedCount {
  key: string;
  n: number;
}

/**
 * The count a row leads with, or undefined when the entry has none worth a
 * place on the row. The key names a unit the client's catalog words.
 */
export function mainCount(entry: FeedEntry): FeedCount | undefined {
  const c = entry.counts ?? {};
  const pick = (key: string): FeedCount | undefined =>
    c[key] !== undefined ? { key, n: c[key] } : undefined;
  switch (entry.kind) {
    case "push":
      return pick("blocks") ?? pick("files");
    case "convergence_run":
      return pick("produced");
    case "review_summary": {
      const n = REVIEW_COUNTS.reduce((sum, k) => sum + (c[k] ?? 0), 0);
      return n > 0 ? { key: "decisions", n } : undefined;
    }
    case "context_change": {
      const kinds = Object.keys(c).filter((k) => !CONTEXT_OPERATIONS.includes(k));
      if (kinds.length === 1) return pick(kinds[0]);
      const n = CONTEXT_OPERATIONS.reduce((sum, k) => sum + (c[k] ?? 0), 0);
      return n > 0 ? { key: "items", n } : undefined;
    }
    case "coverage_milestone":
      return pick("percent");
    default: {
      const first = Object.entries(c).find(([, n]) => n > 0);
      return first ? { key: first[0], n: first[1] } : undefined;
    }
  }
}

/** The counts an entry holds, non-zero, in a stable order for a detail view. */
export function countsOf(entry: FeedEntry): FeedCount[] {
  return Object.entries(entry.counts ?? {})
    .filter(([, n]) => n !== 0)
    .map(([key, n]) => ({ key, n }))
    .sort((a, b) => a.key.localeCompare(b.key));
}

/**
 * Whether an entry records nothing a reader acts on: a run that converged
 * having produced no translation, as the server runs after every change. A
 * run that failed or parked is kept, whatever it produced.
 */
export function isQuietEntry(entry: FeedEntry): boolean {
  return (
    entry.kind === "convergence_run" &&
    (entry.state === "converged" || entry.state === undefined) &&
    (entry.counts?.produced ?? 0) === 0
  );
}

/**
 * One card of the feed: an entry, or a run of entries of one kind by one actor
 * close together in time, newest first. `entries[0]` is the one the card shows.
 */
export interface FeedCard {
  key: string;
  entries: FeedEntry[];
}

/** How close together entries of one actor and kind fold into one card. */
export const FOLD_WINDOW_MS = 30 * 60 * 1000;

/** The kinds whose entries fold together; the others stand alone. */
const FOLDING_KINDS = ["push", "convergence_run", "context_change", "change_set"];

function actorKey(e: FeedEntry): string {
  return `${e.actor.kind}:${e.actor.id}:${e.actor.name ?? ""}`;
}

/**
 * The cards of a page of entries, in the server's order: quiet entries are
 * dropped, and consecutive entries of one folding kind by one actor within
 * FOLD_WINDOW_MS of the card's newest fold into it.
 */
export function foldFeed(entries: readonly FeedEntry[]): FeedCard[] {
  const cards: FeedCard[] = [];
  for (const entry of entries) {
    if (isQuietEntry(entry)) continue;
    const last = cards[cards.length - 1];
    const lead = last?.entries[0];
    if (
      last &&
      lead &&
      FOLDING_KINDS.includes(entry.kind) &&
      lead.kind === entry.kind &&
      actorKey(lead) === actorKey(entry) &&
      Date.parse(lead.at) - Date.parse(entry.at) <= FOLD_WINDOW_MS
    ) {
      last.entries.push(entry);
      continue;
    }
    cards.push({ key: entry.id, entries: [entry] });
  }
  return cards;
}

/**
 * A stable index into a palette of `size` colours for an actor, so one
 * person keeps one colour on every card and every device.
 */
export function paletteIndex(id: string, size: number): number {
  let h = 2166136261;
  for (let i = 0; i < id.length; i++) {
    h ^= id.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return Math.abs(h) % size;
}

/** Up to two initials of a name: "Ada Lindqvist" gives "AL". */
export function initialsOf(name: string): string {
  const parts = name
    .trim()
    .split(/[\s._@-]+/)
    .filter(Boolean);
  if (parts.length === 0) return "?";
  const first = parts[0][0] ?? "";
  const last = parts.length > 1 ? (parts[parts.length - 1][0] ?? "") : (parts[0][1] ?? "");
  return (first + last).toUpperCase();
}

/** How long ago an entry happened, in the unit a compact time shows. */
export type FeedAge =
  | { unit: "now" }
  | { unit: "minutes" | "hours"; n: number }
  | { unit: "yesterday" }
  | { unit: "date"; date: Date };

/** The compact age of `iso` at `now`: now, 5m, 2h, yesterday, or the date. */
export function ageOf(iso: string, now: Date = new Date()): FeedAge {
  const at = new Date(iso);
  const ms = now.getTime() - at.getTime();
  if (Number.isNaN(ms) || ms < 60_000) return { unit: "now" };
  if (ms < 3_600_000) return { unit: "minutes", n: Math.floor(ms / 60_000) };
  const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime();
  if (at.getTime() >= startOfToday) return { unit: "hours", n: Math.floor(ms / 3_600_000) };
  if (at.getTime() >= startOfToday - 86_400_000) return { unit: "yesterday" };
  return { unit: "date", date: at };
}

/** The languages a recipe change added and removed. */
export function targetChanges(entry: FeedEntry): { added: string[]; removed: string[] } {
  const t = entry.diff?.targets;
  if (!t) return { added: [], removed: [] };
  return {
    added: t.after.filter((l) => !t.before.includes(l)),
    removed: t.before.filter((l) => !t.after.includes(l)),
  };
}
