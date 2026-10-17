// State at the window's start against state at the snapshot, for the same
// identities. A net change hides turnover: ten records moving one way and six
// the other read as four. Each pair is its own row.

import { type Atlas, preparedStateAt } from "./atlas";
import type { EvalContext } from "./filter";
import { compareStates } from "./state";

export interface Transition {
  from: string;
  to: string;
  weight: number;
  count: number;
}

export interface TransitionSummary {
  /** Every pair, largest first. */
  transitions: Transition[];
  /** Weight whose state differs between the two instants. */
  changedWeight: number;
  changedCount: number;
  totalWeight: number;
  totalCount: number;
}

/** Group a record set by (state at start, state at snapshot). */
export function transitions(
  atlas: Atlas,
  indices: Iterable<number>,
  ctx: EvalContext,
): TransitionSummary {
  const map = new Map<string, Transition>();
  let changedWeight = 0;
  let changedCount = 0;
  let totalWeight = 0;
  let totalCount = 0;
  for (const i of indices) {
    const p = atlas.records[i];
    const from = preparedStateAt(p, ctx.startMs);
    const to = preparedStateAt(p, ctx.snapshotMs);
    const key = `${from}\u0000${to}`;
    const t = map.get(key) ?? { from, to, weight: 0, count: 0 };
    t.weight += p.weight;
    t.count += 1;
    map.set(key, t);
    totalWeight += p.weight;
    totalCount += 1;
    if (from !== to) {
      changedWeight += p.weight;
      changedCount += 1;
    }
  }
  const list = [...map.values()].sort(
    (a, b) => b.weight - a.weight || compareStates(a.from, b.from) || compareStates(a.to, b.to),
  );
  return { transitions: list, changedWeight, changedCount, totalWeight, totalCount };
}
