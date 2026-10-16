// Two axes crossed. Each cell counts the selection at that pair of values and
// the share of it waiting for a person. A cell nobody occupies has no share:
// an empty cell is not a zero.

import type { Atlas } from "./atlas";
import { type EvalContext, valueOf } from "./filter";
import { preparedStateAt } from "./atlas";
import { NEEDS_PERSON } from "./state";

export interface MatrixCell {
  row: string;
  col: string;
  weight: number;
  count: number;
  attention: number;
  /** attention / weight, or null for an empty cell. */
  share: number | null;
}

export interface Matrix {
  rows: string[];
  cols: string[];
  /** cells[r][c]. */
  cells: MatrixCell[][];
  rowTotals: number[];
  colTotals: number[];
  maxWeight: number;
}

/** Values of an axis in catalogue order, keeping only those in `present` (and any it lacks). */
export function orderedValues(atlas: Atlas, axis: string, present: Set<string>): string[] {
  const listed = atlas.axisById.get(axis)?.values.map((v) => v.id) ?? [];
  const out = listed.filter((v) => present.has(v));
  for (const v of [...present].sort()) if (!out.includes(v)) out.push(v);
  return out;
}

/**
 * Cross `rowAxis` with `colAxis` over a selection. Rows and columns are the
 * values the surrounding cohort takes (pass the selection itself to use only
 * its own), so a filter does not reshuffle the grid.
 */
export function crossMatrix(
  atlas: Atlas,
  selection: readonly number[],
  surrounding: readonly number[],
  rowAxis: string,
  colAxis: string,
  ctx: EvalContext,
): Matrix {
  const rowSet = new Set<string>();
  const colSet = new Set<string>();
  for (const i of surrounding) {
    const p = atlas.records[i];
    rowSet.add(valueOf(p, rowAxis, ctx));
    colSet.add(valueOf(p, colAxis, ctx));
  }
  const rows = orderedValues(atlas, rowAxis, rowSet);
  const cols = orderedValues(atlas, colAxis, colSet);
  const rIndex = new Map(rows.map((v, i) => [v, i]));
  const cIndex = new Map(cols.map((v, i) => [v, i]));
  const cells = rows.map((row) =>
    cols.map((col) => ({
      row,
      col,
      weight: 0,
      count: 0,
      attention: 0,
      share: null as number | null,
    })),
  );
  const rowTotals = rows.map(() => 0);
  const colTotals = cols.map(() => 0);
  for (const i of selection) {
    const p = atlas.records[i];
    const r = rIndex.get(valueOf(p, rowAxis, ctx));
    const c = cIndex.get(valueOf(p, colAxis, ctx));
    if (r === undefined || c === undefined) continue;
    const cell = cells[r][c];
    cell.weight += p.weight;
    cell.count += 1;
    if (NEEDS_PERSON.has(preparedStateAt(p, ctx.snapshotMs))) cell.attention += p.weight;
    rowTotals[r] += p.weight;
    colTotals[c] += p.weight;
  }
  let maxWeight = 0;
  for (const row of cells)
    for (const cell of row) {
      cell.share = cell.weight > 0 ? cell.attention / cell.weight : null;
      if (cell.weight > maxWeight) maxWeight = cell.weight;
    }
  return { rows, cols, cells, rowTotals, colTotals, maxWeight };
}
