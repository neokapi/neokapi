// The over-time raster: one row per value of an axis, one column per time
// bin. Each cell reads the selection's state at the bin's END, not an average
// over the bin. A cell where none of the row's records was recorded yet is
// empty, which is different from a cell where none waits for a person.

import { type Atlas, preparedStateAt } from "./atlas";
import { type EvalContext, valueOf } from "./filter";
import { orderedValues } from "./matrix";
import { ABSENT, NEEDS_PERSON } from "./state";
import { addDays, dayEnd, daysBetween, minDay } from "./time";

export interface RasterBin {
  /** First day of the bin, inclusive. */
  from: string;
  /** Last day of the bin, inclusive. */
  to: string;
  /** The instant the bin's state is read at: the end of `to`. */
  endMs: number;
}

/**
 * Split `[from, to]` into bins: one per day up to a month, one per week up to
 * half a year, else one per 30 days. The last bin ends on `to` and may be
 * shorter than the rest.
 */
export function rasterBins(from: string, to: string): RasterBin[] {
  const days = daysBetween(from, to) + 1;
  const size = days <= 31 ? 1 : days <= 182 ? 7 : 30;
  const bins: RasterBin[] = [];
  for (let d = from; d <= to; d = addDays(d, size)) {
    const end = minDay(addDays(d, size - 1), to);
    bins.push({ from: d, to: end, endMs: dayEnd(end) });
  }
  return bins;
}

export interface RasterCell {
  /** Weight recorded by the bin's end. */
  observed: number;
  /** Of that, weight waiting for a person. */
  attention: number;
  /** attention / observed, or null when nothing was recorded yet. */
  share: number | null;
}

export interface Raster {
  axis: string;
  rows: string[];
  bins: RasterBin[];
  /** cells[row][bin]. */
  cells: RasterCell[][];
}

/**
 * Build the raster for a selection over the query's window. The row a record
 * sits in is read at the snapshot, so a record keeps its row across the bins.
 */
export function buildRaster(
  atlas: Atlas,
  selection: readonly number[],
  axis: string,
  from: string,
  to: string,
  ctx: EvalContext,
): Raster {
  const bins = rasterBins(from, to);
  const rowOf = new Map<number, string>();
  const present = new Set<string>();
  for (const i of selection) {
    const v = valueOf(atlas.records[i], axis, ctx);
    rowOf.set(i, v);
    present.add(v);
  }
  const rows = orderedValues(atlas, axis, present);
  const rIndex = new Map(rows.map((v, i) => [v, i]));
  const cells: RasterCell[][] = rows.map(() =>
    bins.map(() => ({ observed: 0, attention: 0, share: null as number | null })),
  );
  for (const i of selection) {
    const p = atlas.records[i];
    const r = rIndex.get(rowOf.get(i) ?? "");
    if (r === undefined) continue;
    bins.forEach((bin, b) => {
      const state = preparedStateAt(p, bin.endMs);
      if (state === ABSENT) return;
      const cell = cells[r][b];
      cell.observed += p.weight;
      if (NEEDS_PERSON.has(state)) cell.attention += p.weight;
    });
  }
  for (const row of cells)
    for (const cell of row) cell.share = cell.observed > 0 ? cell.attention / cell.observed : null;
  return { axis, rows, bins, cells };
}
