import { useMemo } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import { cn } from "@neokapi/ui-primitives";
import { attentionTint } from "../colors";
import { axisLabel, formatCount, formatShare, valueLabel } from "../labels";
import type { Atlas } from "../model/atlas";
import type { EvalContext, Pick } from "../model/filter";
import { cohort } from "../model/filter";
import { crossMatrix } from "../model/matrix";
import { type AtlasQuery, withRetained } from "../model/query";
import { AxisSelect } from "./AxisSelect";

export interface MatrixViewProps {
  atlas: Atlas;
  query: AtlasQuery;
  ctx: EvalContext;
  foreground: readonly number[];
  preview: ReadonlySet<number> | null;
  onPreview: (picks: Pick[] | null) => void;
  onQuery: (next: AtlasQuery) => void;
}

/**
 * Two axes crossed. A cell shows how much of the selection sits at both
 * values, tinted by the share waiting for a person. A cell nobody occupies is
 * drawn empty with a dash; 0% is a cell with records and none waiting.
 */
export function MatrixView({ atlas, query, ctx, foreground, onPreview, onQuery }: MatrixViewProps) {
  const rowsAxis = query.matrixRows;
  const colsAxis =
    query.matrixCols === rowsAxis
      ? (query.flowAxes.find((a) => a !== rowsAxis) ?? rowsAxis)
      : query.matrixCols;
  const matrix = useMemo(() => {
    const surrounding = cohort(atlas, query, ctx, [rowsAxis, colsAxis]);
    return crossMatrix(atlas, foreground, surrounding, rowsAxis, colsAxis, ctx);
  }, [atlas, query, ctx, foreground, rowsAxis, colsAxis]);

  return (
    <div className="flex flex-col gap-3" data-testid="atlas-matrix">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-[11px] text-muted-foreground">Rows</span>
        <AxisSelect
          atlas={atlas}
          value={rowsAxis}
          onChange={(v) => onQuery({ ...query, matrixRows: v })}
          label={t("Matrix rows", "matrix axis picker")}
        />
        <span className="text-[11px] text-muted-foreground">Columns</span>
        <AxisSelect
          atlas={atlas}
          value={colsAxis}
          onChange={(v) => onQuery({ ...query, matrixCols: v })}
          label={t("Matrix columns", "matrix axis picker")}
        />
      </div>
      <div className="overflow-x-auto">
        <table className="border-separate border-spacing-1 text-[11px]">
          <thead>
            <tr>
              <th className="sr-only">{axisLabel(atlas, rowsAxis)}</th>
              {matrix.cols.map((c) => (
                <th
                  key={c}
                  scope="col"
                  className="max-w-16 truncate px-1 font-medium text-muted-foreground"
                  title={valueLabel(atlas, colsAxis, c)}
                >
                  {valueLabel(atlas, colsAxis, c)}
                </th>
              ))}
              <th scope="col" className="px-1 text-right font-medium text-muted-foreground">
                Total
              </th>
            </tr>
          </thead>
          <tbody>
            {matrix.rows.map((r, ri) => (
              <tr key={r}>
                <th
                  scope="row"
                  className="max-w-40 truncate pr-2 text-right font-normal text-foreground"
                  title={valueLabel(atlas, rowsAxis, r)}
                >
                  {valueLabel(atlas, rowsAxis, r)}
                </th>
                {matrix.cols.map((c, ci) => {
                  const cell = matrix.cells[ri][ci];
                  const empty = cell.weight === 0;
                  const picks: Pick[] = [
                    { axis: rowsAxis, value: r },
                    { axis: colsAxis, value: c },
                  ];
                  const name = empty
                    ? t("{row} and {col}: no records", "matrix cell", {
                        row: valueLabel(atlas, rowsAxis, r),
                        col: valueLabel(atlas, colsAxis, c),
                      })
                    : t("{row} and {col}: {count}, {share} need a person", "matrix cell", {
                        row: valueLabel(atlas, rowsAxis, r),
                        col: valueLabel(atlas, colsAxis, c),
                        count: formatCount(cell.weight),
                        share: formatShare(cell.share),
                      });
                  return (
                    <td key={c} className="p-0">
                      <button
                        type="button"
                        aria-label={name}
                        title={name}
                        disabled={empty}
                        className={cn(
                          "flex h-9 w-16 flex-col items-center justify-center rounded-md tabular-nums outline-none focus-visible:ring-2 focus-visible:ring-ring",
                          empty
                            ? "border border-dashed border-border text-muted-foreground"
                            : "cursor-pointer text-foreground hover:ring-2 hover:ring-foreground/40",
                        )}
                        style={
                          empty
                            ? undefined
                            : {
                                background: attentionTint(cell.share),
                                // Past half, the fill is amber enough to want the dark ink.
                                color:
                                  (cell.share ?? 0) > 0.5 ? "var(--warning-foreground)" : undefined,
                              }
                        }
                        onMouseEnter={() => !empty && onPreview(picks)}
                        onMouseLeave={() => onPreview(null)}
                        onFocus={() => !empty && onPreview(picks)}
                        onBlur={() => onPreview(null)}
                        onClick={(e) => {
                          onPreview(null);
                          onQuery(withRetained(query, picks, e.shiftKey));
                        }}
                      >
                        {empty ? (
                          "–"
                        ) : (
                          <>
                            <span className="font-medium">{formatCount(cell.weight)}</span>
                            <span className="text-[10px] font-mono opacity-80">
                              {formatShare(cell.share)}
                            </span>
                          </>
                        )}
                      </button>
                    </td>
                  );
                })}
                <td className="px-1 text-right text-muted-foreground tabular-nums">
                  {formatCount(matrix.rowTotals[ri])}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-muted-foreground">
        <span className="flex items-center gap-1">
          {[0, 0.5, 1].map((s) => (
            <span
              key={s}
              aria-hidden
              className="inline-block h-2.5 w-5 rounded-[3px]"
              style={{ background: attentionTint(s) }}
            />
          ))}
        </span>
        <span>Tint: share needing a person at the snapshot, 0 to 100%.</span>
        <span>A dash marks a pair no record takes.</span>
      </div>
    </div>
  );
}
