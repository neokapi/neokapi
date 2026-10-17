import { useMemo } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import {
  Button,
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  StatRow,
} from "@neokapi/ui-primitives";
import { ATLAS_STYLE } from "../colors";
import { axisLabel, formatCount, formatDay, valueLabel } from "../labels";
import { type Atlas, DERIVED_AXES } from "../model/atlas";
import { compareQueries } from "../model/compare";
import type { AtlasQuery } from "../model/query";

const A_FILL = "var(--atlas-cat-3)";
const B_FILL = "var(--atlas-cat-1)";

export interface CompareViewProps {
  atlas: Atlas;
  /** A: the pinned query. */
  pinned: AtlasQuery;
  /** B: the current query. */
  query: AtlasQuery;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Make A the current query again. */
  onRestore: () => void;
  /** Swap A and B. */
  onSwap: () => void;
  onExport?: () => void;
}

function SideHeader({
  side,
  query,
  count,
  fill,
}: {
  side: "A" | "B";
  query: AtlasQuery;
  count: number;
  fill: string;
}) {
  return (
    <div className="flex flex-col gap-0.5">
      <span className="flex items-center gap-1.5 text-sm font-medium">
        <span aria-hidden className="size-2.5 rounded-full" style={{ background: fill }} />
        {t("{side}: {count} records", "compare side heading", { side, count: formatCount(count) })}
      </span>
      <span className="text-[11px] text-muted-foreground tabular-nums">
        {t("{from} to {to}, snapshot {asOf}", "compare side dates", {
          from: formatDay(query.from),
          to: formatDay(query.to, true),
          asOf: formatDay(query.asOf),
        })}
      </span>
    </div>
  );
}

/**
 * A against B in one frame: the same category order on both sides and one
 * count scale, so bars compare directly. Membership (only in A, only in B) is
 * counted apart from state changes among the records both share.
 */
export function CompareView({
  atlas,
  pinned,
  query,
  open,
  onOpenChange,
  onRestore,
  onSwap,
  onExport,
}: CompareViewProps) {
  const cmp = useMemo(
    () => compareQueries(atlas, pinned, query, query.flowAxes),
    [atlas, pinned, query],
  );
  const dateOf = (axis: string, q: AtlasQuery) =>
    axis === DERIVED_AXES.statusStart ? q.from : q.asOf;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="context-atlas max-h-[90vh] overflow-y-auto sm:max-w-5xl"
        data-testid="atlas-compare"
      >
        {/* The dialog renders in a portal, outside the atlas root that defines the palette. */}
        <style>{ATLAS_STYLE}</style>
        <DialogHeader>
          <DialogTitle>Compare A and B</DialogTitle>
        </DialogHeader>
        <div className="flex flex-wrap items-start justify-between gap-4">
          <SideHeader side="A" query={pinned} count={cmp.a.count} fill={A_FILL} />
          <SideHeader side="B" query={query} count={cmp.b.count} fill={B_FILL} />
        </div>
        <div className="grid gap-3 [grid-template-columns:repeat(auto-fit,minmax(200px,1fr))]">
          {cmp.axes.map((ax) => (
            <section
              key={ax.axis}
              className="flex flex-col gap-2 rounded-lg border p-3"
              aria-label={axisLabel(atlas, ax.axis)}
            >
              <div>
                <h4 className="text-sm font-medium">{axisLabel(atlas, ax.axis)}</h4>
                {ax.temporal && (
                  <span className="text-[11px] text-muted-foreground tabular-nums">
                    {t("A at {a}, B at {b}", "compare: the dates a time-read axis uses", {
                      a: formatDay(dateOf(ax.axis, pinned)),
                      b: formatDay(dateOf(ax.axis, query)),
                    })}
                  </span>
                )}
              </div>
              <ul className="flex flex-col gap-2">
                {ax.rows.map((row) => (
                  <li key={row.value} className="flex flex-col gap-0.5">
                    <span className="flex items-baseline justify-between gap-2 text-[12px]">
                      <span className="truncate" title={valueLabel(atlas, ax.axis, row.value)}>
                        {valueLabel(atlas, ax.axis, row.value)}
                      </span>
                      <span className="shrink-0 font-mono text-[11px] text-muted-foreground tabular-nums">
                        {formatCount(row.a)} → {formatCount(row.b)}
                      </span>
                    </span>
                    {(
                      [
                        [row.a, A_FILL],
                        [row.b, B_FILL],
                      ] as const
                    ).map(([w, fill], i) => (
                      <span key={i} className="block h-1.5 rounded-full bg-muted">
                        <span
                          className="block h-1.5 rounded-full"
                          style={{
                            width: `${cmp.maxWeight > 0 ? (w / cmp.maxWeight) * 100 : 0}%`,
                            background: fill,
                          }}
                        />
                      </span>
                    ))}
                  </li>
                ))}
              </ul>
            </section>
          ))}
        </div>
        <StatRow
          items={[
            {
              id: "changed",
              value: cmp.changedState,
              label: t("of {common} in both changed state", "compare count", {
                common: formatCount(cmp.common),
              }),
              tone: cmp.changedState > 0 ? "warning" : "default",
            },
            { id: "only-a", value: cmp.onlyA, label: t("only in A", "compare count") },
            { id: "only-b", value: cmp.onlyB, label: t("only in B", "compare count") },
            {
              id: "needs-a",
              value: cmp.a.attention,
              label: t("need a person in A", "compare count"),
              tone: "muted",
            },
            {
              id: "needs-b",
              value: cmp.b.attention,
              label: t("need a person in B", "compare count"),
              tone: "muted",
            },
          ]}
        />
        <DialogFooter className="gap-2 sm:justify-start">
          <Button variant="outline" onClick={onRestore}>
            Restore A
          </Button>
          <Button variant="outline" onClick={onSwap}>
            Swap A and B
          </Button>
          {onExport && (
            <Button variant="outline" onClick={onExport}>
              Export investigation
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
