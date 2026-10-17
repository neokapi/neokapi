// Events per day by kind, on the recorded or the effective clock: the strip
// the time brush draws over the whole span.

import type { Atlas } from "./atlas";
import type { EventClock } from "./query";
import { dayOf, dayRange } from "./time";

/** The groups an event kind falls in, in stacking order. */
export const EVENT_GROUPS = ["added", "decided", "closed", "window", "other"] as const;
export type EventGroup = (typeof EVENT_GROUPS)[number];

const GROUP_OF: Record<string, EventGroup> = {
  observe: "added",
  correct: "added",
  import: "added",
  "term-added": "added",
  edit: "added",
  signal: "added",
  keep: "decided",
  establish: "decided",
  widen: "decided",
  decision: "decided",
  drop: "closed",
  withdraw: "closed",
  reset: "closed",
  "window-opens": "window",
  "window-closes": "window",
};

/** The group an event kind is counted under. */
export function eventGroup(kind: string): EventGroup {
  return GROUP_OF[kind] ?? "other";
}

export interface EventHistogram {
  clock: EventClock;
  days: string[];
  /** counts[day][group], groups in EVENT_GROUPS order. */
  counts: number[][];
  /** Events per day, summed over groups. */
  totals: number[];
  max: number;
  /** Events whose instant falls outside the span (none, for well-formed data). */
  outside: number;
}

/** Count events per day of the atlas's span, by group, on one clock. */
export function eventHistogram(atlas: Atlas, clock: EventClock): EventHistogram {
  const days = dayRange(atlas.firstDay, atlas.lastDay);
  const index = new Map(days.map((d, i) => [d, i]));
  const counts = days.map(() => EVENT_GROUPS.map(() => 0));
  let outside = 0;
  for (const e of atlas.events) {
    const ms = clock === "effective" ? e.effectiveMs : e.recordedMs;
    if (Number.isNaN(ms)) {
      outside += 1;
      continue;
    }
    const i = index.get(dayOf(ms));
    if (i === undefined) {
      outside += 1;
      continue;
    }
    counts[i][EVENT_GROUPS.indexOf(eventGroup(e.kind))] += 1;
  }
  const totals = counts.map((row) => row.reduce((s, n) => s + n, 0));
  return { clock, days, counts, totals, max: Math.max(0, ...totals), outside };
}

/** Events whose instant on `clock` falls in `[fromMs, toMs]`. */
export function eventsInWindow(
  atlas: Atlas,
  clock: EventClock,
  fromMs: number,
  toMs: number,
): number {
  let n = 0;
  for (const e of atlas.events) {
    const ms = clock === "effective" ? e.effectiveMs : e.recordedMs;
    if (ms >= fromMs && ms <= toMs) n += 1;
  }
  return n;
}
