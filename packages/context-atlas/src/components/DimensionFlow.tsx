import { useMemo, useRef, useState, type KeyboardEvent, type MouseEvent } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import {
  Button,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Switch,
  cn,
} from "@neokapi/ui-primitives";
import { AxisSelect } from "./AxisSelect";
import { ArrowLeft, ArrowRight, Plus, X } from "lucide-react";
import { NEUTRAL, RIBBON_GROUND, attentionFill, changeFill, valueFill } from "../colors";
import { axisLabel, formatCount, formatDay, lensLabel, valueLabel } from "../labels";
import { type Atlas, DERIVED_AXES } from "../model/atlas";
import type { EvalContext, Pick } from "../model/filter";
import {
  type FlowAgg,
  type FlowLayout,
  type FlowNode,
  type FlowRibbon,
  aggregateFlow,
  attentionDelta,
  attentionShare,
  layoutFlow,
} from "../model/flow";
import {
  type AtlasQuery,
  FLOW_LENSES,
  type FlowLens,
  MAX_FLOW_AXES,
  MIN_FLOW_AXES,
  addFlowAxis,
  moveFlowAxis,
  removeFlowAxis,
  setFlowAxis,
  withRetained,
} from "../model/query";
import { useElementWidth } from "./hooks";

const NODE_W = 12;
const LABEL_W = 210;
const MIN_COL_W = 240;
const PLOT_H = 360;
const PAD_X = 4;

export interface DimensionFlowProps {
  atlas: Atlas;
  query: AtlasQuery;
  ctx: EvalContext;
  /** The selection, by record index. */
  foreground: readonly number[];
  /** The surrounding cohort (the displayed axes' filters released). */
  universe: readonly number[];
  /** The identity set being previewed, if any. */
  preview: ReadonlySet<number> | null;
  /** Hover or focus on a mark: preview these picks; null clears. */
  onPreview: (picks: Pick[] | null) => void;
  onQuery: (next: AtlasQuery) => void;
}

function lensFill(lens: FlowLens, atlas: Atlas, firstAxis: string, agg: FlowAgg): string {
  switch (lens) {
    case "origin":
      return agg.origin === null ? NEUTRAL : valueFill(atlas, firstAxis, agg.origin);
    case "attention":
      return attentionFill(attentionShare(agg));
    case "change":
      return changeFill(attentionDelta(agg));
  }
}

function band(x0: number, y0: number, x1: number, y1: number, th: number): string {
  const xm = (x0 + x1) / 2;
  return [
    `M ${x0} ${y0}`,
    `C ${xm} ${y0} ${xm} ${y1} ${x1} ${y1}`,
    `L ${x1} ${y1 + th}`,
    `C ${xm} ${y1 + th} ${xm} ${y0 + th} ${x0} ${y0 + th}`,
    "Z",
  ].join(" ");
}

function truncate(s: string, max: number): string {
  return s.length > max ? `${s.slice(0, max - 1)}…` : s;
}

/**
 * The dimension flow. Each column lists one axis's values; a ribbon joins two
 * ADJACENT columns and is as thick as the records that take both values. The
 * faint ribbons behind are the surrounding context: the same records with the
 * displayed axes' filters released. Hover or focus a node or a ribbon to
 * trace those records through every column; click to retain them, shift-click
 * to add to what the column retains.
 */
export function DimensionFlow({
  atlas,
  query,
  ctx,
  foreground,
  universe,
  preview,
  onPreview,
  onQuery,
}: DimensionFlowProps) {
  const [wrapRef, width] = useElementWidth<HTMLDivElement>(960);
  const axes = query.flowAxes;
  const colW = Math.max(
    MIN_COL_W,
    (width - NODE_W - LABEL_W - PAD_X * 2) / Math.max(1, axes.length - 1),
  );
  const layout: FlowLayout = useMemo(
    () =>
      layoutFlow(atlas, {
        axes,
        universe: query.surrounding ? universe : foreground,
        foreground,
        ctx,
        height: PLOT_H,
        columnWidth: colW,
        minNodeHeight: 16,
        gap: 6,
        keep: query.filters,
        // A record (or a unit) never draws taller than this, so a few fill only part of the height.
        maxScale: PLOT_H / (query.measure === "records" ? 20 : 200),
      }),
    [atlas, axes, universe, foreground, ctx, colW, query.surrounding, query.filters, query.measure],
  );
  const previewAgg = useMemo(
    () => (preview ? aggregateFlow(atlas, preview, axes, ctx) : null),
    [atlas, preview, axes, ctx],
  );
  const svgW = PAD_X * 2 + (axes.length - 1) * colW + NODE_W + LABEL_W;
  const svgH = Math.max(layout.height, 60) + 8;

  // Roving focus over the nodes: arrows move within and across columns.
  const [focusKey, setFocusKey] = useState<string | null>(null);
  const nodeRefs = useRef(new Map<string, SVGGElement>());
  const firstKey = layout.columns[0]?.nodes[0]?.key ?? null;
  const tabKey =
    focusKey && layout.columns.some((c) => c.nodes.some((n) => n.key === focusKey))
      ? focusKey
      : firstKey;

  const moveFocus = (node: FlowNode, e: KeyboardEvent) => {
    const col = layout.columns[node.column];
    const i = col.nodes.findIndex((n) => n.key === node.key);
    let target: FlowNode | undefined;
    if (e.key === "ArrowDown") target = col.nodes[i + 1];
    else if (e.key === "ArrowUp") target = col.nodes[i - 1];
    else if (e.key === "ArrowRight" || e.key === "ArrowLeft") {
      const next = layout.columns[node.column + (e.key === "ArrowRight" ? 1 : -1)];
      if (next && next.nodes.length > 0) {
        const mid = node.y + node.height / 2;
        target = next.nodes.reduce((best, n) =>
          Math.abs(n.y + n.height / 2 - mid) < Math.abs(best.y + best.height / 2 - mid) ? n : best,
        );
      }
    } else return false;
    if (target) {
      setFocusKey(target.key);
      nodeRefs.current.get(target.key)?.focus();
    }
    return true;
  };

  const retain = (picks: Pick[], add: boolean) => {
    onPreview(null);
    onQuery(withRetained(query, picks, add));
  };

  const surroundingLabel = query.surrounding;
  const subtitle = (axis: string) => {
    if (axis === DERIVED_AXES.statusStart)
      return t("at {day}", "flow column date", { day: formatDay(query.from) });
    if (
      axis === DERIVED_AXES.statusSnapshot ||
      axis === DERIVED_AXES.lifetime ||
      axis === DERIVED_AXES.age
    ) {
      return t("at {day}", "flow column date", { day: formatDay(query.asOf) });
    }
    return surroundingLabel
      ? t("selected / surrounding", "flow column counts")
      : t("selected", "flow column counts");
  };

  const first = axes[0];

  const nodeLabel = (n: FlowNode) =>
    t("{axis}: {value}, {selected} of {surrounding}", "flow node accessible name", {
      axis: axisLabel(atlas, n.axis),
      value: valueLabel(atlas, n.axis, n.value),
      selected: formatCount(n.fg.weight),
      surrounding: formatCount(n.all.weight),
    });
  const ribbonLabel = (r: FlowRibbon) =>
    t("{a} and {b}: {selected} of {surrounding}", "flow ribbon accessible name", {
      a: valueLabel(atlas, axes[r.column], r.from),
      b: valueLabel(atlas, axes[r.column + 1], r.to),
      selected: formatCount(r.fg.weight),
      surrounding: formatCount(r.all.weight),
    });

  const xOf = (column: number) => PAD_X + column * colW;
  const dim = preview !== null;

  return (
    <div className="flex flex-col gap-3" data-testid="atlas-flow">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
        <div className="flex items-center gap-2">
          <span className="text-[11px] text-muted-foreground">Colour</span>
          <Select
            value={query.lens}
            onValueChange={(v) => onQuery({ ...query, lens: v as FlowLens })}
          >
            <SelectTrigger size="sm" aria-label={t("Colour ribbons by", "flow lens")}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {FLOW_LENSES.map((lens) => (
                <SelectItem key={lens} value={lens}>
                  {lensLabel(lens)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="flex items-center gap-2">
          <Switch
            id="atlas-surrounding"
            checked={query.surrounding}
            onCheckedChange={(v) => onQuery({ ...query, surrounding: v })}
          />
          <Label
            htmlFor="atlas-surrounding"
            className="text-[11px] font-normal text-muted-foreground"
          >
            Surrounding context
          </Label>
        </div>
        <Button
          variant="outline"
          size="xs"
          className="ml-auto"
          disabled={axes.length >= MAX_FLOW_AXES}
          onClick={() => onQuery(addFlowAxis(query, atlas))}
        >
          <Plus aria-hidden />
          Column
        </Button>
      </div>

      <div ref={wrapRef} className="w-full overflow-x-auto pb-1">
        <div style={{ width: svgW }} className="relative">
          <div className="relative h-14" role="group" aria-label={t("Flow columns", "flow")}>
            {axes.map((axis, i) => (
              <div
                key={axis}
                className="absolute top-0 flex flex-col gap-0.5"
                style={{ left: xOf(i), width: Math.min(colW - 12, 224) }}
              >
                <div className="flex items-center gap-0.5">
                  <AxisSelect
                    atlas={atlas}
                    value={axis}
                    onChange={(v) => onQuery(setFlowAxis(query, i, v))}
                    label={t("Column {n} axis", "flow column picker", { n: i + 1 })}
                    className="min-w-0 flex-1 font-medium"
                  />
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    aria-label={t("Move {axis} left", "flow column", {
                      axis: axisLabel(atlas, axis),
                    })}
                    disabled={i === 0}
                    onClick={() => onQuery(moveFlowAxis(query, i, -1))}
                  >
                    <ArrowLeft aria-hidden />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    aria-label={t("Move {axis} right", "flow column", {
                      axis: axisLabel(atlas, axis),
                    })}
                    disabled={i === axes.length - 1}
                    onClick={() => onQuery(moveFlowAxis(query, i, 1))}
                  >
                    <ArrowRight aria-hidden />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    aria-label={t("Remove {axis}", "flow column", { axis: axisLabel(atlas, axis) })}
                    disabled={axes.length <= MIN_FLOW_AXES}
                    onClick={() => onQuery(removeFlowAxis(query, i))}
                  >
                    <X aria-hidden />
                  </Button>
                </div>
                <span className="pl-1 text-[11px] text-muted-foreground">{subtitle(axis)}</span>
              </div>
            ))}
          </div>

          <svg
            width={svgW}
            height={svgH}
            className="block"
            role="group"
            aria-label={t("Dimension flow", "flow chart")}
            onMouseLeave={() => onPreview(null)}
          >
            <g transform="translate(0 4)">
              {query.surrounding &&
                layout.ribbons.map((r) => (
                  <path
                    key={`bg-${r.key}`}
                    d={band(
                      xOf(r.column) + NODE_W,
                      r.sourceY,
                      xOf(r.column + 1),
                      r.targetY,
                      r.thickness,
                    )}
                    fill={RIBBON_GROUND}
                    opacity={0.55}
                    pointerEvents="none"
                  />
                ))}
              {layout.ribbons.map((r) => {
                const picks: Pick[] = [
                  { axis: axes[r.column], value: r.from },
                  { axis: axes[r.column + 1], value: r.to },
                ];
                const prev = previewAgg?.ribbons.get(r.key);
                const fill = lensFill(query.lens, atlas, first, r.fg);
                return (
                  <g key={r.key}>
                    {r.fgThickness > 0 && (
                      <path
                        d={band(
                          xOf(r.column) + NODE_W,
                          r.sourceY,
                          xOf(r.column + 1),
                          r.targetY,
                          r.fgThickness,
                        )}
                        fill={fill}
                        opacity={dim ? 0.18 : 0.62}
                        className="transition-opacity"
                        pointerEvents="none"
                      />
                    )}
                    {prev && prev.weight > 0 && (
                      <path
                        d={band(
                          xOf(r.column) + NODE_W,
                          r.sourceY,
                          xOf(r.column + 1),
                          r.targetY,
                          prev.weight * layout.scale,
                        )}
                        fill={lensFill(query.lens, atlas, first, prev)}
                        opacity={0.9}
                        pointerEvents="none"
                      />
                    )}
                    {/* The hit area: the whole band, so a thin selection is still easy to reach. */}
                    <path
                      d={band(
                        xOf(r.column) + NODE_W,
                        r.sourceY,
                        xOf(r.column + 1),
                        r.targetY,
                        Math.max(r.thickness, 3),
                      )}
                      fill="transparent"
                      className="cursor-pointer"
                      aria-label={ribbonLabel(r)}
                      onMouseEnter={() => onPreview(picks)}
                      onMouseLeave={() => onPreview(null)}
                      onClick={(e: MouseEvent) => retain(picks, e.shiftKey)}
                    >
                      <title>{ribbonLabel(r)}</title>
                    </path>
                  </g>
                );
              })}
              {layout.columns.map((col) =>
                col.nodes.map((n) => {
                  const x = xOf(n.column);
                  const fgH = n.fg.weight > 0 ? Math.max(2, n.fg.weight * layout.scale) : 0;
                  const prev = previewAgg?.nodes.get(n.key);
                  const prevH =
                    prev && prev.weight > 0 ? Math.max(2, prev.weight * layout.scale) : 0;
                  const retained = query.filters[n.axis]?.includes(n.value) ?? false;
                  const fill = valueFill(atlas, n.axis, n.value);
                  const picks: Pick[] = [{ axis: n.axis, value: n.value }];
                  const label = valueLabel(atlas, n.axis, n.value);
                  return (
                    <g
                      key={n.key}
                      ref={(el) => {
                        if (el) nodeRefs.current.set(n.key, el);
                        else nodeRefs.current.delete(n.key);
                      }}
                      role="button"
                      tabIndex={n.key === tabKey ? 0 : -1}
                      aria-label={nodeLabel(n)}
                      aria-pressed={retained}
                      data-testid={`flow-node-${n.axis}-${n.value}`}
                      className="cursor-pointer outline-none [&:focus-visible>.atlas-node-ring]:stroke-ring"
                      onMouseEnter={() => onPreview(picks)}
                      onMouseLeave={() => onPreview(null)}
                      onFocus={() => {
                        setFocusKey(n.key);
                        onPreview(picks);
                      }}
                      onBlur={() => onPreview(null)}
                      onClick={(e) => retain(picks, e.shiftKey)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter" || e.key === " ") {
                          e.preventDefault();
                          retain(picks, e.shiftKey);
                        } else if (moveFocus(n, e)) e.preventDefault();
                        else if (e.key === "Escape") onPreview(null);
                      }}
                    >
                      <title>{nodeLabel(n)}</title>
                      <rect
                        x={x}
                        y={n.y}
                        width={NODE_W}
                        height={n.height}
                        rx={3}
                        fill={fill}
                        opacity={fill === "transparent" ? 1 : n.fg.weight > 0 ? 0.28 : 0.14}
                        stroke={fill === "transparent" ? "var(--muted-foreground)" : undefined}
                        strokeDasharray={fill === "transparent" ? "3 2" : undefined}
                      />
                      {fgH > 0 && (
                        <rect
                          x={x}
                          y={n.y}
                          width={NODE_W}
                          height={Math.min(fgH, n.height)}
                          rx={3}
                          fill={fill}
                          opacity={dim ? 0.35 : 1}
                        />
                      )}
                      {prevH > 0 && (
                        <rect
                          x={x}
                          y={n.y}
                          width={NODE_W}
                          height={Math.min(prevH, n.height)}
                          rx={3}
                          fill={fill}
                        />
                      )}
                      <rect
                        className="atlas-node-ring"
                        x={x - 2}
                        y={n.y - 2}
                        width={NODE_W + 4}
                        height={n.height + 4}
                        rx={4}
                        fill="none"
                        stroke={retained ? "var(--foreground)" : "transparent"}
                        strokeWidth={1.5}
                      />
                      {/* Hit area that includes the label. */}
                      <rect
                        x={x}
                        y={n.y}
                        width={Math.min(colW - 24, LABEL_W)}
                        height={Math.max(n.height, 16)}
                        fill="transparent"
                      />
                      <text
                        x={x + NODE_W + 6}
                        y={n.y + Math.min(n.height, 24) / 2 + 4}
                        fontSize={12}
                        fill={n.fg.weight > 0 ? "var(--foreground)" : "var(--muted-foreground)"}
                        fontWeight={retained ? 600 : 400}
                        stroke="var(--card)"
                        strokeWidth={3}
                        strokeLinejoin="round"
                        paintOrder="stroke"
                      >
                        {truncate(label, 17)}
                      </text>
                      <text
                        x={x + NODE_W + 6 + 182}
                        y={n.y + Math.min(n.height, 24) / 2 + 4}
                        fontSize={11}
                        textAnchor="end"
                        fill="var(--muted-foreground)"
                        stroke="var(--card)"
                        strokeWidth={3}
                        strokeLinejoin="round"
                        paintOrder="stroke"
                        className="tabular-nums"
                      >
                        {preview
                          ? `${formatCount(prev?.weight ?? 0)} / ${formatCount(n.all.weight)}`
                          : query.surrounding
                            ? `${formatCount(n.fg.weight)} / ${formatCount(n.all.weight)}`
                            : formatCount(n.fg.weight)}
                      </text>
                    </g>
                  );
                }),
              )}
            </g>
          </svg>
        </div>
      </div>
      <FlowLegend lens={query.lens} atlas={atlas} firstAxis={first} />
    </div>
  );
}

function FlowLegend({
  lens,
  atlas,
  firstAxis,
}: {
  lens: FlowLens;
  atlas: Atlas;
  firstAxis: string;
}) {
  if (lens === "origin") {
    return (
      <p className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-muted-foreground">
        <span>
          {t(
            "Ribbons take the colour of their {axis} value; grey where they mix several.",
            "flow legend",
            {
              axis: axisLabel(atlas, firstAxis),
            },
          )}
        </span>
        <span>Width counts records; hover traces them, click retains them.</span>
      </p>
    );
  }
  const stops = lens === "attention" ? [0, 0.5, 1] : [-0.5, 0, 0.5];
  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-muted-foreground">
      <span className="flex items-center gap-1.5">
        {stops.map((s) => (
          <span key={s} className="flex items-center gap-1">
            <span
              aria-hidden
              className={cn("inline-block h-2.5 w-5 rounded-[3px]")}
              style={{ background: lens === "attention" ? attentionFill(s) : changeFill(s) }}
            />
            <span className="tabular-nums">
              {lens === "attention"
                ? t("{pct}%", "legend stop", { pct: Math.round(s * 100) })
                : t("{pp} pp", "legend stop", {
                    pp: `${s > 0 ? "+" : s < 0 ? "−" : ""}${Math.abs(Math.round(s * 100))}`,
                  })}
            </span>
          </span>
        ))}
      </span>
      <span>
        {lens === "attention"
          ? t("Share waiting for a person at the snapshot.", "flow legend")
          : t(
              "Change in that share from the window's start; beyond 50 pp the colour holds.",
              "flow legend",
            )}
      </span>
    </div>
  );
}
