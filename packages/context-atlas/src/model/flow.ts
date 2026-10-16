// The dimension flow: columns of axis values joined by ribbons. A ribbon is
// the intersection of two ADJACENT columns' values: the records that take
// value a on one axis and value b on the next. Ribbons are not paths; a band
// that looks continuous across three columns says nothing about a single
// record's tuple. The view traces identities across columns by previewing a
// record set and repainting every column from it.

import { type Atlas, DERIVED_AXES, preparedStateAt } from "./atlas";
import { type EvalContext, valueOf } from "./filter";
import { ABSENT, NEEDS_PERSON } from "./state";

/** What a set of records amounts to at a node or a ribbon. */
export interface FlowAgg {
  /** Sum of weights. Widths and labels use this. */
  weight: number;
  /** Number of records. */
  count: number;
  /** Weight waiting for a person at the snapshot. */
  attention: number;
  /** Weight recorded by the window's start. */
  observedStart: number;
  /** Weight waiting for a person at the window's start. */
  attentionStart: number;
  /**
   * The first column's value when every record here shares one; `null` when
   * they come from several.
   */
  origin: string | null;
}

export function emptyAgg(): FlowAgg {
  return { weight: 0, count: 0, attention: 0, observedStart: 0, attentionStart: 0, origin: null };
}

/** Share waiting for a person at the snapshot; null with nothing to divide by. */
export function attentionShare(a: FlowAgg): number | null {
  return a.weight > 0 ? a.attention / a.weight : null;
}

/**
 * How the share waiting for a person moved from the window's start to the
 * snapshot, over the same identities: those recorded at each instant form
 * that instant's denominator. Null when either instant has none recorded.
 */
export function attentionDelta(a: FlowAgg): number | null {
  if (a.weight <= 0 || a.observedStart <= 0) return null;
  return a.attention / a.weight - a.attentionStart / a.observedStart;
}

export function nodeKey(column: number, value: string): string {
  return `${column}\u0000${value}`;
}

export function ribbonKey(column: number, from: string, to: string): string {
  return `${column}\u0000${from}\u0000${to}`;
}

/** Aggregates for every node and every adjacent-pair ribbon. */
export interface FlowAggregates {
  nodes: Map<string, FlowAgg>;
  ribbons: Map<string, FlowAgg>;
  total: FlowAgg;
}

function add(
  agg: FlowAgg,
  w: number,
  attn: boolean,
  obsStart: boolean,
  attnStart: boolean,
  origin: string,
  first: boolean,
) {
  agg.weight += w;
  agg.count += 1;
  if (attn) agg.attention += w;
  if (obsStart) agg.observedStart += w;
  if (attnStart) agg.attentionStart += w;
  if (first) agg.origin = origin;
  else if (agg.origin !== origin) agg.origin = null;
}

/**
 * Sum a record set into the flow's nodes and ribbons. Every record lands in
 * exactly one node per column and one ribbon per boundary, so each column and
 * each boundary sums to the same total.
 */
export function aggregateFlow(
  atlas: Atlas,
  indices: Iterable<number>,
  axes: readonly string[],
  ctx: EvalContext,
): FlowAggregates {
  const nodes = new Map<string, FlowAgg>();
  const ribbons = new Map<string, FlowAgg>();
  const total = emptyAgg();
  const seen = new Set<FlowAgg>();
  const touch = (map: Map<string, FlowAgg>, key: string): [FlowAgg, boolean] => {
    let agg = map.get(key);
    if (!agg) {
      agg = emptyAgg();
      map.set(key, agg);
    }
    const first = !seen.has(agg);
    seen.add(agg);
    return [agg, first];
  };
  let firstTotal = true;
  for (const i of indices) {
    const p = atlas.records[i];
    const snap = preparedStateAt(p, ctx.snapshotMs);
    const start = preparedStateAt(p, ctx.startMs);
    const attn = NEEDS_PERSON.has(snap);
    const obsStart = start !== ABSENT;
    const attnStart = NEEDS_PERSON.has(start);
    const values = axes.map((axis) =>
      axis === DERIVED_AXES.statusSnapshot
        ? snap
        : axis === DERIVED_AXES.statusStart
          ? start
          : valueOf(p, axis, ctx),
    );
    const origin = values[0] ?? "";
    add(total, p.weight, attn, obsStart, attnStart, origin, firstTotal);
    firstTotal = false;
    for (let c = 0; c < values.length; c++) {
      const [agg, first] = touch(nodes, nodeKey(c, values[c]));
      add(agg, p.weight, attn, obsStart, attnStart, origin, first);
      if (c + 1 < values.length) {
        const [r, rFirst] = touch(ribbons, ribbonKey(c, values[c], values[c + 1]));
        add(r, p.weight, attn, obsStart, attnStart, origin, rFirst);
      }
    }
  }
  return { nodes, ribbons, total };
}

export interface FlowNode {
  key: string;
  column: number;
  axis: string;
  value: string;
  y: number;
  height: number;
  /** The surrounding cohort here (the selection when surrounding context is off). */
  all: FlowAgg;
  /** The selection here. */
  fg: FlowAgg;
}

export interface FlowRibbon {
  key: string;
  /** The left column; the ribbon joins it to the next. */
  column: number;
  from: string;
  to: string;
  /** Top of the band where it leaves the left node, and where it enters the right one. */
  sourceY: number;
  targetY: number;
  /** Band thickness for the surrounding cohort, and for the selection inside it. */
  thickness: number;
  fgThickness: number;
  all: FlowAgg;
  fg: FlowAgg;
}

export interface FlowColumn {
  axis: string;
  x: number;
  nodes: FlowNode[];
}

export interface FlowLayout {
  columns: FlowColumn[];
  ribbons: FlowRibbon[];
  /** Pixels per unit of weight; ribbon thickness is weight times scale. */
  scale: number;
  width: number;
  height: number;
  all: FlowAgg;
  fg: FlowAgg;
}

export interface FlowOptions {
  axes: readonly string[];
  /** The surrounding cohort; pass the selection itself to draw no surrounding context. */
  universe: readonly number[];
  /** The selection; a subset of the universe. */
  foreground: readonly number[];
  ctx: EvalContext;
  /** Plot height available to a column. */
  height: number;
  /** Horizontal distance between columns. */
  columnWidth: number;
  /** Smallest node height, so a sparse value stays visible and selectable. */
  minNodeHeight?: number;
  /** Gap between nodes in a column. */
  gap?: number;
  /** Values to keep in a column even when nothing takes them (retained filters). */
  keep?: Readonly<Record<string, readonly string[]>>;
  /** Largest pixels per unit of weight, so a handful of records does not fill the height. */
  maxScale?: number;
}

/** The scale at which a column's nodes, at their minimum heights, fit the height. */
function fitScale(weights: number[], height: number, minH: number, gap: number): number {
  const total = weights.reduce((s, w) => s + w, 0);
  const avail = height - gap * Math.max(0, weights.length - 1);
  if (total <= 0) return 0;
  if (avail <= minH * weights.length) return Math.max(0, avail) / total / 4;
  // f(s) = Σ max(minH, w·s) grows with s; find the largest s with f(s) ≤ avail.
  let lo = 0;
  let hi = avail / total;
  for (let k = 0; k < 40; k++) {
    const mid = (lo + hi) / 2;
    const used = weights.reduce((s, w) => s + Math.max(minH, w * mid), 0);
    if (used <= avail) lo = mid;
    else hi = mid;
  }
  return lo;
}

/**
 * Lay out the flow. Node heights share one scale (with a minimum height, so
 * small values stay reachable; a node's height is therefore not exactly
 * proportional, while every ribbon's thickness is). Ribbons leave a node in
 * the order of the nodes they reach, and enter one in the order of the nodes
 * they come from.
 */
export function layoutFlow(atlas: Atlas, opts: FlowOptions): FlowLayout {
  const minH = opts.minNodeHeight ?? 14;
  const gap = opts.gap ?? 8;
  const all = aggregateFlow(atlas, opts.universe, opts.axes, opts.ctx);
  const fg = aggregateFlow(atlas, opts.foreground, opts.axes, opts.ctx);

  const columnValues = opts.axes.map((axisId, c) => {
    const axis = atlas.axisById.get(axisId);
    const keep = new Set(opts.keep?.[axisId] ?? []);
    const ordered = axis ? axis.values.map((v) => v.id) : [];
    // Values the records take that the catalogue does not list still show.
    for (const key of all.nodes.keys()) {
      const [col, value] = key.split("\u0000");
      if (Number(col) === c && !ordered.includes(value)) ordered.push(value);
    }
    return ordered.filter((v) => (all.nodes.get(nodeKey(c, v))?.weight ?? 0) > 0 || keep.has(v));
  });

  const scale = Math.min(
    ...columnValues.map((values, c) =>
      fitScale(
        values.map((v) => all.nodes.get(nodeKey(c, v))?.weight ?? 0),
        opts.height,
        minH,
        gap,
      ),
    ),
    Number.POSITIVE_INFINITY,
  );
  const s = Math.min(Number.isFinite(scale) ? scale : 0, opts.maxScale ?? Number.POSITIVE_INFINITY);

  const columns: FlowColumn[] = columnValues.map((values, c) => {
    let y = 0;
    const nodes = values.map((value) => {
      const key = nodeKey(c, value);
      const a = all.nodes.get(key) ?? emptyAgg();
      const height = Math.max(minH, a.weight * s);
      const node: FlowNode = {
        key,
        column: c,
        axis: opts.axes[c],
        value,
        y,
        height,
        all: a,
        fg: fg.nodes.get(key) ?? emptyAgg(),
      };
      y += height + gap;
      return node;
    });
    return { axis: opts.axes[c], x: c * opts.columnWidth, nodes };
  });

  const ribbons: FlowRibbon[] = [];
  for (let c = 0; c + 1 < columns.length; c++) {
    const left = columns[c].nodes;
    const right = columns[c + 1].nodes;
    const outY = new Map(left.map((n) => [n.value, n.y]));
    const inY = new Map(right.map((n) => [n.value, n.y]));
    const pairs: FlowRibbon[] = [];
    for (const l of left) {
      for (const r of right) {
        const key = ribbonKey(c, l.value, r.value);
        const a = all.ribbons.get(key);
        if (!a || a.weight <= 0) continue;
        const f = fg.ribbons.get(key) ?? emptyAgg();
        pairs.push({
          key,
          column: c,
          from: l.value,
          to: r.value,
          sourceY: 0,
          targetY: 0,
          thickness: a.weight * s,
          fgThickness: f.weight * s,
          all: a,
          fg: f,
        });
      }
    }
    // Leaving: in the order of the right column. Entering: in the order of the left.
    for (const rb of pairs) {
      rb.sourceY = outY.get(rb.from) ?? 0;
      outY.set(rb.from, rb.sourceY + rb.thickness);
    }
    const leftOrder = new Map(left.map((n, i) => [n.value, i]));
    const byTarget = [...pairs].sort(
      (a, b) => (leftOrder.get(a.from) ?? 0) - (leftOrder.get(b.from) ?? 0),
    );
    const rightOrder = new Map(right.map((n, i) => [n.value, i]));
    byTarget.sort((a, b) => (rightOrder.get(a.to) ?? 0) - (rightOrder.get(b.to) ?? 0));
    for (const rb of byTarget) {
      rb.targetY = inY.get(rb.to) ?? 0;
      inY.set(rb.to, rb.targetY + rb.thickness);
    }
    ribbons.push(...pairs);
  }

  const height = Math.max(
    0,
    ...columns.map((col) => col.nodes.reduce((m, n) => Math.max(m, n.y + n.height), 0)),
  );
  return {
    columns,
    ribbons,
    scale: s,
    width: Math.max(0, columns.length - 1) * opts.columnWidth,
    height,
    all: all.total,
    fg: fg.total,
  };
}
