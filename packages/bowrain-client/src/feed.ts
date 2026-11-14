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
