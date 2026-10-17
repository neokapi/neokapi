// A record's state at an instant, and the fixed order and tone states are
// drawn in.

import type { AtlasRecord } from "../types";
import { instantMs, isUnknownInstant } from "./time";

/** The state of a record before its first history entry. */
export const ABSENT = "absent";

/** The state of a record that carries no history at all. */
export const UNKNOWN = "unknown";

/**
 * Every state the contract names, in the order a status axis lists them:
 * settled first, then what waits for a person, then the content ladder,
 * then what was closed, then what cannot be read.
 */
export const STATE_ORDER = [
  "established",
  "suggested",
  "contested",
  "stale",
  "translated",
  "draft",
  "not-started",
  "withdrawn",
  "dropped",
  "reset",
  UNKNOWN,
  ABSENT,
] as const;

/**
 * States that wait for a person's decision: a suggestion nobody kept or
 * dropped, a disagreement, a decision whose source moved, and a target that
 * waits for approval.
 */
export const NEEDS_PERSON: ReadonlySet<string> = new Set([
  "suggested",
  "contested",
  "stale",
  "translated",
]);

/** How a state is drawn. Maps onto the shared judgement colours. */
export type StateTone =
  | "settled" // established: the success green
  | "pending" // suggested: the info blue
  | "attention" // contested, stale: the warning amber
  | "middle" // translated: the neutral middle rung
  | "start" // draft, not started: muted
  | "closed" // withdrawn, dropped, reset: muted
  | "unknown" // no history: muted
  | "absent"; // not recorded yet: an outline only

const TONES: Record<string, StateTone> = {
  established: "settled",
  suggested: "pending",
  contested: "attention",
  stale: "attention",
  translated: "middle",
  draft: "start",
  "not-started": "start",
  withdrawn: "closed",
  dropped: "closed",
  reset: "closed",
  [UNKNOWN]: "unknown",
  [ABSENT]: "absent",
};

/** The tone a state is drawn in; a state this build does not know is `middle`. */
export function stateTone(state: string): StateTone {
  return TONES[state] ?? "middle";
}

/** Sort key for a state: known states in STATE_ORDER, others after, by name. */
export function stateRank(state: string): number {
  const i = (STATE_ORDER as readonly string[]).indexOf(state);
  return i === -1 ? STATE_ORDER.length - 2 : i;
}

/** Compare two states by STATE_ORDER, unknown names alphabetically between. */
export function compareStates(a: string, b: string): number {
  return stateRank(a) - stateRank(b) || a.localeCompare(b);
}

/**
 * The state a record held at instant `t` (milliseconds): the last history
 * entry with `at <= t`, or `absent` before the first. A record with no history
 * reads as `unknown` from the moment it was recorded (from always, when that
 * is not known either), because the contract promises at least one entry and
 * a record without one says nothing about its state.
 */
export function stateAt(record: AtlasRecord, t: number): string {
  const history = record.history;
  if (!history || history.length === 0) {
    if (isUnknownInstant(record.recorded)) return UNKNOWN;
    return instantMs(record.recorded) <= t ? UNKNOWN : ABSENT;
  }
  let state = ABSENT;
  for (const entry of history) {
    const at = instantMs(entry.at);
    if (Number.isNaN(at) || at > t) break;
    state = entry.state;
  }
  return state;
}

/** Whether the record exists in the record at instant `t`. */
export function observedAt(record: AtlasRecord, t: number): boolean {
  return stateAt(record, t) !== ABSENT;
}
