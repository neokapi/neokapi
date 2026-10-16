// A against B: two queries over the same atlas, read in one frame. Category
// order is fixed by the catalogue and both sides share one count scale, so a
// longer bar is a larger count on either side. Identities in both are
// "common"; membership changes (only in A, only in B) are counted apart from
// state changes among the common ones.

import { type Atlas, preparedStateAt } from "./atlas";
import { type EvalContext, cohort, evalContext, valueOf } from "./filter";
import { orderedValues } from "./matrix";
import type { AtlasQuery } from "./query";
import { NEEDS_PERSON } from "./state";

export interface CompareSide {
  query: AtlasQuery;
  ctx: EvalContext;
  indices: number[];
  weight: number;
  count: number;
  attention: number;
}

export interface CompareRow {
  value: string;
  a: number;
  b: number;
}

export interface CompareAxis {
  axis: string;
  /** Whether the axis reads each side at that side's own dates. */
  temporal: boolean;
  rows: CompareRow[];
}

export interface Comparison {
  a: CompareSide;
  b: CompareSide;
  axes: CompareAxis[];
  /** The largest count on any row of either side: the shared scale. */
  maxWeight: number;
  /** Records in both. */
  common: number;
  onlyA: number;
  onlyB: number;
  /** Common records whose state at the snapshot differs between A and B. */
  changedState: number;
}

function side(atlas: Atlas, query: AtlasQuery): CompareSide {
  const ctx = evalContext(atlas, query);
  const indices = cohort(atlas, query, ctx);
  let weight = 0;
  let attention = 0;
  for (const i of indices) {
    const p = atlas.records[i];
    weight += p.weight;
    if (NEEDS_PERSON.has(preparedStateAt(p, ctx.snapshotMs))) attention += p.weight;
  }
  return { query, ctx, indices, weight, count: indices.length, attention };
}

/** Compare two queries along `axes` (by default, B's flow columns). */
export function compareQueries(
  atlas: Atlas,
  qa: AtlasQuery,
  qb: AtlasQuery,
  axes: readonly string[] = qb.flowAxes,
): Comparison {
  const a = side(atlas, qa);
  const b = side(atlas, qb);
  let maxWeight = 0;
  const out: CompareAxis[] = axes.map((axis) => {
    const ca = new Map<string, number>();
    const cb = new Map<string, number>();
    for (const i of a.indices) {
      const v = valueOf(atlas.records[i], axis, a.ctx);
      ca.set(v, (ca.get(v) ?? 0) + atlas.records[i].weight);
    }
    for (const i of b.indices) {
      const v = valueOf(atlas.records[i], axis, b.ctx);
      cb.set(v, (cb.get(v) ?? 0) + atlas.records[i].weight);
    }
    const values = orderedValues(atlas, axis, new Set([...ca.keys(), ...cb.keys()]));
    const rows = values.map((value) => {
      const row = { value, a: ca.get(value) ?? 0, b: cb.get(value) ?? 0 };
      maxWeight = Math.max(maxWeight, row.a, row.b);
      return row;
    });
    return { axis, temporal: atlas.axisById.get(axis)?.temporal ?? false, rows };
  });

  const inA = new Set(a.indices);
  const inB = new Set(b.indices);
  let common = 0;
  let changedState = 0;
  for (const i of inA) {
    if (!inB.has(i)) continue;
    common += 1;
    const p = atlas.records[i];
    if (preparedStateAt(p, a.ctx.snapshotMs) !== preparedStateAt(p, b.ctx.snapshotMs)) {
      changedState += 1;
    }
  }
  return {
    a,
    b,
    axes: out,
    maxWeight,
    common,
    onlyA: inA.size - common,
    onlyB: inB.size - common,
    changedState,
  };
}
