import { useMemo } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import { cn } from "@neokapi/ui-primitives";
import { attentionTint } from "../colors";
import { formatCount, formatDay, formatShare, valueLabel } from "../labels";
import type { Atlas } from "../model/atlas";
import type { EvalContext, Pick } from "../model/filter";
import { type AtlasQuery, withRetained, withWindow } from "../model/query";
import { buildRaster } from "../model/raster";
import { AxisSelect } from "./AxisSelect";

export interface RasterViewProps {
  atlas: Atlas;
  query: AtlasQuery;
  ctx: EvalContext;
  foreground: readonly number[];
  onPreview: (picks: Pick[] | null) => void;
  onQuery: (next: AtlasQuery) => void;
}

/**
 * Over time: one row per value, one column per bin of the window. Each cell
 * reads the selection's state at the END of its bin. A cell where nothing was
 * recorded yet is drawn empty, apart from a cell where nothing waits.
 */
export function RasterView({ atlas, query, ctx, foreground, onPreview, onQuery }: RasterViewProps) {
  const raster = useMemo(
    () => buildRaster(atlas, foreground, query.rasterAxis, query.from, query.to, ctx),
    [atlas, foreground, query.rasterAxis, query.from, query.to, ctx],
  );
  const every = Math.max(1, Math.ceil(raster.bins.length / 12));

  return (
    <div className="flex flex-col gap-3" data-testid="atlas-raster">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-[11px] text-muted-foreground">Rows</span>
        <AxisSelect
          atlas={atlas}
          value={query.rasterAxis}
          dataOnly
          onChange={(v) => onQuery({ ...query, rasterAxis: v })}
          label={t("Raster rows", "over-time axis picker")}
        />
        <span className="text-[11px] text-muted-foreground">
          {raster.bins.length > 0 && raster.bins[0].from === raster.bins[0].to
            ? t("One column per day", "raster bin size")
            : t("One column per {days} days", "raster bin size", {
                days:
                  Math.round(
                    (Date.parse(raster.bins[0]?.to ?? "") -
                      Date.parse(raster.bins[0]?.from ?? "")) /
                      86_400_000,
                  ) + 1,
              })}
        </span>
      </div>
      <div className="overflow-x-auto">
        <table
          className={cn(
            "border-separate border-spacing-[3px] text-[11px]",
            raster.bins.length <= 40 && "w-full",
          )}
        >
          <thead>
            <tr>
              <th className="sr-only">{t("Value", "raster row header")}</th>
              {raster.bins.map((b, i) => (
                <th
                  key={b.from}
                  scope="col"
                  className="min-w-7 px-0 text-left font-normal text-muted-foreground tabular-nums"
                >
                  {i % every === 0 ? formatDay(b.from) : ""}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {raster.rows.map((row, ri) => (
              <tr key={row}>
                <th
                  scope="row"
                  className="max-w-40 truncate pr-2 text-right font-normal text-foreground"
                  title={valueLabel(atlas, query.rasterAxis, row)}
                >
                  {valueLabel(atlas, query.rasterAxis, row)}
                </th>
                {raster.bins.map((bin, bi) => {
                  const cell = raster.cells[ri][bi];
                  const empty = cell.share === null;
                  const name = empty
                    ? t("{row}, {from} to {to}: nothing recorded yet", "raster cell", {
                        row: valueLabel(atlas, query.rasterAxis, row),
                        from: formatDay(bin.from),
                        to: formatDay(bin.to),
                      })
                    : t(
                        "{row}, state at the end of {to}: {attention} of {observed} need a person",
                        "raster cell",
                        {
                          row: valueLabel(atlas, query.rasterAxis, row),
                          to: formatDay(bin.to, true),
                          attention: formatCount(cell.attention),
                          observed: formatCount(cell.observed),
                        },
                      );
                  const picks: Pick[] = [{ axis: query.rasterAxis, value: row }];
                  return (
                    <td key={bin.from} className="p-0">
                      <button
                        type="button"
                        aria-label={name}
                        title={name}
                        className={cn(
                          "block h-6 w-full min-w-7 cursor-pointer rounded-[4px] outline-none focus-visible:ring-2 focus-visible:ring-ring hover:ring-2 hover:ring-foreground/40",
                          empty && "border border-dashed border-border",
                        )}
                        style={empty ? undefined : { background: attentionTint(cell.share) }}
                        onMouseEnter={() => onPreview(picks)}
                        onMouseLeave={() => onPreview(null)}
                        onFocus={() => onPreview(picks)}
                        onBlur={() => onPreview(null)}
                        onClick={(e) => {
                          onPreview(null);
                          onQuery(
                            withWindow(withRetained(query, picks, e.shiftKey), bin.from, bin.to),
                          );
                        }}
                      >
                        <span className="sr-only">{empty ? "–" : formatShare(cell.share)}</span>
                      </button>
                    </td>
                  );
                })}
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
        <span>Share needing a person at each column's last day.</span>
        <span className="flex items-center gap-1">
          <span
            aria-hidden
            className="inline-block h-2.5 w-5 rounded-[3px] border border-dashed border-border"
          />
          Nothing recorded yet
        </span>
        <span>Click a cell to retain its row and window.</span>
      </div>
    </div>
  );
}
