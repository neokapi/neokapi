// Pure timeline transforms for the concept-view timeline panel (Apache-2.0):
// sort events chronologically, bucket them by calendar day, and — for sources
// with no revision log — synthesise a minimal core timeline from a concept's
// createdAt/updatedAt and its terms' validity windows. No React here so the
// ordering and bucketing rules are unit-tested directly.

import type { Concept, TimelineEvent } from "./types";

export interface TimelineDay {
  /** Stable calendar-day key, YYYY-MM-DD (or "undated"). */
  key: string;
  /** A friendly date label for the day header. */
  label: string;
  events: TimelineEvent[];
}

/** The YYYY-MM-DD calendar key for an instant (UTC-stable for ISO inputs). */
export function dayKey(iso: string): string {
  if (/^\d{4}-\d{2}-\d{2}/.test(iso)) return iso.slice(0, 10);
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "undated";
  return d.toISOString().slice(0, 10);
}

function dayLabel(key: string): string {
  if (key === "undated") return "Undated";
  const d = new Date(`${key}T12:00:00Z`);
  if (Number.isNaN(d.getTime())) return key;
  return d.toLocaleDateString(undefined, { day: "numeric", month: "long", year: "numeric" });
}

/**
 * Sort events chronologically. `order` controls direction: "asc" tells the
 * story from the beginning, "desc" puts the latest activity first. Stable for
 * equal timestamps, so the output is deterministic.
 */
export function sortTimeline(
  events: TimelineEvent[],
  order: "asc" | "desc" = "desc",
): TimelineEvent[] {
  const dir = order === "asc" ? 1 : -1;
  return events
    .map((e, i) => ({ e, i }))
    .sort((a, b) => {
      if (a.e.at !== b.e.at) return a.e.at < b.e.at ? -dir : dir;
      return (a.i - b.i) * dir;
    })
    .map(({ e }) => e);
}

/** Sort events, then bucket them into per-day groups (same order). */
export function buildTimeline(
  events: TimelineEvent[],
  order: "asc" | "desc" = "desc",
): TimelineDay[] {
  const sorted = sortTimeline(events, order);
  const groups: TimelineDay[] = [];
  const byKey = new Map<string, TimelineDay>();
  for (const event of sorted) {
    const key = dayKey(event.at);
    let group = byKey.get(key);
    if (!group) {
      group = { key, label: dayLabel(key), events: [] };
      byKey.set(key, group);
      groups.push(group);
    }
    group.events.push(event);
  }
  return groups;
}

/**
 * Synthesise a minimal CORE timeline from a concept alone, for sources with no
 * revision log: a "created" event, an "updated" event when it differs, and a
 * "valid-from" event per distinct term validity window. Returned unsorted —
 * pass through buildTimeline. Empty when the concept tracks no timestamps.
 */
export function synthesizeTimeline(concept: Concept): TimelineEvent[] {
  const events: TimelineEvent[] = [];
  if (concept.createdAt) {
    events.push({ kind: "create", at: concept.createdAt, summary: "Concept created" });
  }
  if (concept.updatedAt && concept.updatedAt !== concept.createdAt) {
    events.push({ kind: "revision", at: concept.updatedAt, summary: "Concept updated" });
  }
  const seen = new Set<string>();
  for (const term of concept.terms) {
    const from = term.validity?.validFrom;
    if (!from) continue;
    const dedupe = `${from}:${term.locale}:${term.text}`;
    if (seen.has(dedupe)) continue;
    seen.add(dedupe);
    events.push({
      kind: "status",
      at: from,
      summary: `${term.text} valid in ${term.locale}`,
      ref: term.locale,
    });
  }
  return events;
}
