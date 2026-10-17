// Who is in the cohort: the records a query selects, the surrounding records a
// view draws behind them, and the identity sets a hover previews.

import {
  type Atlas,
  type PreparedRecord,
  DERIVED_AXES,
  coordinateOf,
  preparedStateAt,
} from "./atlas";
import type { AtlasQuery } from "./query";
import { ABSENT } from "./state";
import { DAY_MS, dayEnd, dayStart } from "./time";

/** The instants a query resolves against. */
export interface EvalContext {
  /** The window's first instant; the state "at the start" is read here. */
  startMs: number;
  /** The window's last instant. */
  endMs: number;
  /** The snapshot: the last instant of the `asOf` day. */
  snapshotMs: number;
  /** Ids of records an event touched inside the window, on the query's clock. */
  active: ReadonlySet<string>;
}

/** Resolve a query's days to instants, and find the records the window touched. */
export function evalContext(atlas: Atlas, q: AtlasQuery): EvalContext {
  const startMs = dayStart(q.from);
  const endMs = dayEnd(q.to);
  const active = new Set<string>();
  for (const e of atlas.events) {
    if (!e.record) continue;
    const at = q.clock === "effective" ? e.effectiveMs : e.recordedMs;
    if (at >= startMs && at <= endMs) active.add(e.record);
  }
  return { startMs, endMs, snapshotMs: dayEnd(q.asOf), active };
}

/** Where the snapshot falls in a record's lifetime window. */
export function lifetimeAt(p: PreparedRecord, t: number): string {
  const hasFrom = !Number.isNaN(p.validFromMs);
  const hasTo = !Number.isNaN(p.validToMs);
  if (!hasFrom && !hasTo) return "open";
  if (hasFrom && t < p.validFromMs) return "upcoming";
  // valid_to is exclusive.
  if (hasTo && t >= p.validToMs) return "expired";
  return "in-force";
}

/** Time since a record was recorded, at instant `t`, in four bands. */
export function ageAt(p: PreparedRecord, t: number): string {
  if (Number.isNaN(p.recordedMs)) return "unknown";
  const days = (t - p.recordedMs) / DAY_MS;
  if (days < 7) return "lt-7d";
  if (days < 30) return "7-30d";
  if (days < 90) return "30-90d";
  return "gt-90d";
}

/** The value a record takes on any axis of the catalogue, under a context. */
export function valueOf(p: PreparedRecord, axis: string, ctx: EvalContext): string {
  switch (axis) {
    case DERIVED_AXES.statusStart:
      return preparedStateAt(p, ctx.startMs);
    case DERIVED_AXES.statusSnapshot:
      return preparedStateAt(p, ctx.snapshotMs);
    case DERIVED_AXES.lifetime:
      return lifetimeAt(p, ctx.snapshotMs);
    case DERIVED_AXES.age:
      return ageAt(p, ctx.snapshotMs);
    case DERIVED_AXES.activity:
      return ctx.active.has(p.record.id) ? "changed" : "quiet";
    default:
      return coordinateOf(p, axis);
  }
}

/**
 * Whether a record passes the query's filters: in the explicit id set when
 * there is one, and on every filtered axis one of the retained values (OR
 * within an axis, AND across axes). Axes in `release` are not applied.
 */
export function matches(
  p: PreparedRecord,
  q: AtlasQuery,
  ctx: EvalContext,
  release?: ReadonlySet<string>,
): boolean {
  if (q.ids.length > 0 && !q.ids.includes(p.record.id)) return false;
  for (const [axis, values] of Object.entries(q.filters)) {
    if (values.length === 0 || release?.has(axis)) continue;
    if (!values.includes(valueOf(p, axis, ctx))) return false;
  }
  return true;
}

/**
 * The records a query selects: those recorded by the snapshot that pass its
 * filters, by index. With `release`, those axes' filters are lifted; that is
 * the surrounding cohort a view draws behind the selection, and the cohort a
 * facet counts against.
 */
export function cohort(
  atlas: Atlas,
  q: AtlasQuery,
  ctx: EvalContext = evalContext(atlas, q),
  release?: Iterable<string>,
): number[] {
  const released = release ? new Set(release) : undefined;
  const idSet = q.ids.length > 0 ? new Set(q.ids) : undefined;
  // ids are checked through the set; the filters alone go to `matches`.
  const filtersOnly: AtlasQuery = idSet ? { ...q, ids: [] } : q;
  const out: number[] = [];
  for (const p of atlas.records) {
    if (idSet && !idSet.has(p.record.id)) continue;
    if (preparedStateAt(p, ctx.snapshotMs) === ABSENT) continue;
    if (!matches(p, filtersOnly, ctx, released)) continue;
    out.push(p.index);
  }
  return out;
}

/** The sum of weights over a set of record indices. */
export function weightOf(atlas: Atlas, indices: Iterable<number>): number {
  let w = 0;
  for (const i of indices) w += atlas.records[i].weight;
  return w;
}

/** Weight and record count per value of one axis over a set of records. */
export function facetCounts(
  atlas: Atlas,
  indices: readonly number[],
  axis: string,
  ctx: EvalContext,
): Map<string, { weight: number; count: number }> {
  const out = new Map<string, { weight: number; count: number }>();
  for (const i of indices) {
    const p = atlas.records[i];
    const v = valueOf(p, axis, ctx);
    const cell = out.get(v) ?? { weight: 0, count: 0 };
    cell.weight += p.weight;
    cell.count += 1;
    out.set(v, cell);
  }
  return out;
}

/** One condition of a preview: the record takes `value` on `axis`. */
export interface Pick {
  axis: string;
  value: string;
}

/**
 * The identity set a hover previews: the records among `indices` that meet
 * the picks the way a filter does, one of the picked values on each picked
 * axis. A preview never changes the query; the view repaints every column
 * with this set.
 */
export function previewSet(
  atlas: Atlas,
  indices: readonly number[],
  picks: readonly Pick[],
  ctx: EvalContext,
): Set<number> {
  const byAxis = new Map<string, Set<string>>();
  for (const pk of picks) {
    const values = byAxis.get(pk.axis) ?? new Set<string>();
    values.add(pk.value);
    byAxis.set(pk.axis, values);
  }
  const out = new Set<number>();
  for (const i of indices) {
    const p = atlas.records[i];
    let ok = true;
    for (const [axis, values] of byAxis) {
      if (!values.has(valueOf(p, axis, ctx))) {
        ok = false;
        break;
      }
    }
    if (ok) out.add(i);
  }
  return out;
}
