import { useMemo, useState } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import { SimpleTooltip, ViewTab, ViewTabGroup, cn } from "@neokapi/ui-primitives";
import { Info } from "lucide-react";
import { attentionTint } from "../colors";
import { axisLabel, formatCount, formatShare, valueLabel } from "../labels";
import { type Atlas, preparedStateAt } from "../model/atlas";
import { type EvalContext, type Pick, cohort, valueOf } from "../model/filter";
import { type AtlasQuery, withRetained } from "../model/query";
import { NEEDS_PERSON } from "../model/state";
import { layoutTiles, mapPlaces } from "../model/tiles";
import { AxisSelect } from "./AxisSelect";

/** The geo value that holds every country (`axis.World` on the host). */
const WORLD = "world";

const CELL = 46;

/** Keeps a label legible over the hatched fill of a tile held through a group. */
const HALO = {
  stroke: "var(--background)",
  strokeWidth: 3,
  strokeLinejoin: "round",
  paintOrder: "stroke",
} as const;
const TILE = 42;

export interface MapViewProps {
  atlas: Atlas;
  query: AtlasQuery;
  ctx: EvalContext;
  foreground: readonly number[];
  preview: ReadonlySet<number> | null;
  onPreview: (picks: Pick[] | null) => void;
  onQuery: (next: AtlasQuery) => void;
}

interface Tally {
  weight: number;
  attention: number;
  around: number;
}

/**
 * The map: one square tile per place, set on a grid near where the place
 * lies. Tile size is the same for every place, so land area never inflates a
 * count. A group or region (the Nordics, Europe) is listed beside the grid
 * with its own count, and its countries are drawn hatched: a rule scoped to
 * the group holds there without sitting there. Hovering the group lights them.
 */
export function MapView({
  atlas,
  query,
  ctx,
  foreground,
  preview,
  onPreview,
  onQuery,
}: MapViewProps) {
  const geoAxes = atlas.axes.filter(
    (a) =>
      a.kind === "geo" &&
      a.origin !== "view" &&
      a.values.some((v) => v.centroid || (v.members?.length ?? 0) > 0),
  );
  const [axisId, setAxisId] = useState(geoAxes[0]?.id ?? "");
  const axis = atlas.axisById.get(axisId) ?? geoAxes[0];
  const [measure, setMeasure] = useState<"count" | "attention">("count");

  const places = useMemo(() => mapPlaces(axis?.values ?? []), [axis]);
  const grid = useMemo(() => layoutTiles(places.places), [places]);

  const tallies = useMemo(() => {
    const map = new Map<string, Tally>();
    if (!axis) return map;
    const get = (v: string) => {
      let x = map.get(v);
      if (!x) map.set(v, (x = { weight: 0, attention: 0, around: 0 }));
      return x;
    };
    for (const i of cohort(atlas, query, ctx, [axis.id])) {
      get(valueOf(atlas.records[i], axis.id, ctx)).around += atlas.records[i].weight;
    }
    for (const i of foreground) {
      const p = atlas.records[i];
      const x = get(valueOf(p, axis.id, ctx));
      x.weight += p.weight;
      if (NEEDS_PERSON.has(preparedStateAt(p, ctx.snapshotMs))) x.attention += p.weight;
    }
    return map;
  }, [atlas, query, ctx, foreground, axis]);

  const previewCounts = useMemo(() => {
    if (!preview || !axis) return null;
    const m = new Map<string, number>();
    for (const i of preview) {
      const v = valueOf(atlas.records[i], axis.id, ctx);
      m.set(v, (m.get(v) ?? 0) + atlas.records[i].weight);
    }
    return m;
  }, [preview, atlas, axis, ctx]);

  if (!axis) return null;
  const max = Math.max(1, ...[...tallies.values()].map((x) => x.weight));
  const placed = new Set(axis.values.filter((v) => v.centroid).map((v) => v.id));
  const isValue = new Set(axis.values.map((v) => v.id));
  // What reaches a place through the groups and regions that hold it.
  const through = (id: string) => {
    let weight = 0;
    for (const g of places.within.get(id) ?? []) weight += tallies.get(g)?.weight ?? 0;
    return weight;
  };
  const lit = (id: string) =>
    previewCounts === null ||
    (previewCounts.get(id) ?? 0) > 0 ||
    (places.within.get(id) ?? []).some((g) => (previewCounts.get(g) ?? 0) > 0);
  const placeName = (id: string) =>
    isValue.has(id) ? valueLabel(atlas, axis.id, id) : (places.names.get(id) ?? id.toUpperCase());
  const unplaced = axis.values.filter(
    (v) => !placed.has(v.id) && (tallies.get(v.id)?.around ?? 0) > 0,
  );
  const fillFor = (x: Tally | undefined) => {
    if (!x || x.weight === 0) return "transparent";
    if (measure === "attention") return attentionTint(x.weight > 0 ? x.attention / x.weight : null);
    return `color-mix(in oklab, var(--primary) ${Math.round(18 + (x.weight / max) * 72)}%, var(--background))`;
  };
  const inkFor = (x: Tally | undefined) => {
    if (!x || x.weight === 0) return "var(--muted-foreground)";
    if (measure === "attention")
      return x.attention / x.weight > 0.5 ? "var(--warning-foreground)" : "var(--foreground)";
    return x.weight / max > 0.45 ? "var(--primary-foreground)" : "var(--foreground)";
  };
  const nameOf = (id: string, x: Tally | undefined) => {
    const groups = places.within.get(id) ?? [];
    const own = t("{place}: {count} selected, {share} need a person", "map tile", {
      place: placeName(id),
      count: formatCount(x?.weight ?? 0),
      share: formatShare(x && x.weight > 0 ? x.attention / x.weight : null),
    });
    if (groups.length === 0) return own;
    return t("{own}; {count} more hold here through {groups}", "map tile held through a group", {
      own,
      count: formatCount(through(id)),
      groups: groups.map((g) => valueLabel(atlas, axis.id, g)).join(", "),
    });
  };
  const act = (id: string) => ({
    onMouseEnter: () => onPreview([{ axis: axis.id, value: id }]),
    onMouseLeave: () => onPreview(null),
    onFocus: () => onPreview([{ axis: axis.id, value: id }]),
    onBlur: () => onPreview(null),
    onClick: (e: { shiftKey: boolean }) => {
      onPreview(null);
      onQuery(withRetained(query, [{ axis: axis.id, value: id }], e.shiftKey));
    },
  });
  // A country tile keeps everything that holds there: the country itself, when
  // records sit at it, and every group or region that holds it.
  const tileAct = (id: string) => {
    // World holds every country; it is kept with the rest but never hatched,
    // since a stripe on every tile would say nothing.
    const picks = [
      ...(isValue.has(id) ? [{ axis: axis.id, value: id }] : []),
      ...(places.within.get(id) ?? []).map((g) => ({ axis: axis.id, value: g })),
      ...(isValue.has(WORLD) ? [{ axis: axis.id, value: WORLD }] : []),
    ];
    return {
      onMouseEnter: () => onPreview(picks),
      onMouseLeave: () => onPreview(null),
      onFocus: () => onPreview(picks),
      onBlur: () => onPreview(null),
      onClick: (e: { shiftKey: boolean }) => {
        onPreview(null);
        if (picks.length > 0) onQuery(withRetained(query, picks, e.shiftKey));
      },
    };
  };

  return (
    <div className="flex flex-col gap-3" data-testid="atlas-map">
      <div className="flex flex-wrap items-center gap-2">
        {geoAxes.length > 1 && (
          <AxisSelect
            atlas={atlas}
            value={axis.id}
            kind="geo"
            onChange={setAxisId}
            label={t("Map axis", "map axis picker")}
          />
        )}
        {geoAxes.length === 1 && (
          <span className="text-sm font-medium">{axisLabel(atlas, axis.id)}</span>
        )}
        <ViewTabGroup aria-label={t("Tile colour", "map measure")}>
          <ViewTab active={measure === "count"} onClick={() => setMeasure("count")}>
            Selected
          </ViewTab>
          <ViewTab active={measure === "attention"} onClick={() => setMeasure("attention")}>
            Needs a person
          </ViewTab>
        </ViewTabGroup>
        <SimpleTooltip
          content={t(
            "Each place is one tile, set near where it lies. Tile area is not geographic area, so a large country counts the same as a small one.",
            "map info",
          )}
        >
          <button
            type="button"
            className="inline-flex size-6 items-center justify-center rounded-md text-muted-foreground hover:bg-muted"
            aria-label={t("About the map's tiles", "map info button")}
          >
            <Info aria-hidden className="size-4" />
          </button>
        </SimpleTooltip>
      </div>
      <div className="flex flex-wrap items-start gap-6">
        <div className="overflow-x-auto">
          <svg
            width={grid.cols * CELL}
            height={grid.rows * CELL}
            role="group"
            aria-label={t("{axis} tiles", "map chart", { axis: axisLabel(atlas, axis.id) })}
          >
            <defs>
              <pattern
                id="atlas-map-through"
                width="6"
                height="6"
                patternUnits="userSpaceOnUse"
                patternTransform="rotate(45)"
              >
                <rect width="6" height="6" fill="var(--background)" />
                <rect
                  width="2.5"
                  height="6"
                  fill="color-mix(in oklab, var(--primary) 35%, var(--background))"
                />
              </pattern>
            </defs>
            {grid.tiles.map((tile) => {
              const x = tallies.get(tile.id);
              const retained = query.filters[axis.id]?.includes(tile.id) ?? false;
              const dimmed = !lit(tile.id);
              const held = (x?.weight ?? 0) === 0 && through(tile.id) > 0;
              const handlers = tileAct(tile.id);
              return (
                <g
                  key={tile.id}
                  role="button"
                  tabIndex={0}
                  aria-label={nameOf(tile.id, x)}
                  aria-pressed={retained}
                  className={cn(
                    "cursor-pointer outline-none [&:focus-visible>rect]:stroke-ring",
                    dimmed && "opacity-35",
                  )}
                  transform={`translate(${tile.col * CELL} ${tile.row * CELL})`}
                  onMouseEnter={handlers.onMouseEnter}
                  onMouseLeave={handlers.onMouseLeave}
                  onFocus={handlers.onFocus}
                  onBlur={handlers.onBlur}
                  onClick={handlers.onClick}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      e.preventDefault();
                      handlers.onClick(e);
                    }
                  }}
                >
                  <title>{nameOf(tile.id, x)}</title>
                  <rect
                    width={TILE}
                    height={TILE}
                    rx={6}
                    fill={held ? "url(#atlas-map-through)" : fillFor(x)}
                    stroke={retained ? "var(--foreground)" : "var(--border)"}
                    strokeWidth={retained ? 2 : 1}
                    strokeDasharray={(x && x.weight > 0) || held ? undefined : "3 3"}
                  />
                  <text
                    x={TILE / 2}
                    y={17}
                    textAnchor="middle"
                    fontSize={12}
                    fontWeight={600}
                    fill={held ? "var(--foreground)" : inkFor(x)}
                    {...(held ? HALO : {})}
                  >
                    {tile.id.toUpperCase()}
                  </text>
                  <text
                    x={TILE / 2}
                    y={32}
                    textAnchor="middle"
                    fontSize={10}
                    fill={held ? "var(--foreground)" : inkFor(x)}
                    className="font-mono tabular-nums"
                    {...(held ? HALO : {})}
                  >
                    {held
                      ? t("+{count}", "map tile: records holding through a group", {
                          count: formatCount(through(tile.id)),
                        })
                      : formatCount(x?.weight ?? 0)}
                  </text>
                </g>
              );
            })}
          </svg>
        </div>
        {unplaced.length > 0 && (
          <ul
            className="flex min-w-48 flex-col gap-1"
            aria-label={t("Places without a position", "map list")}
          >
            {unplaced.map((v) => {
              const x = tallies.get(v.id);
              const retained = query.filters[axis.id]?.includes(v.id) ?? false;
              const h = act(v.id);
              return (
                <li key={v.id}>
                  <button
                    type="button"
                    aria-pressed={retained}
                    className={cn(
                      "flex w-full items-center justify-between gap-3 rounded-md px-2 py-1 text-left text-sm hover:bg-muted",
                      retained && "ring-1 ring-foreground",
                    )}
                    onMouseEnter={h.onMouseEnter}
                    onMouseLeave={h.onMouseLeave}
                    onFocus={h.onFocus}
                    onBlur={h.onBlur}
                    onClick={h.onClick}
                  >
                    <span className="truncate">{valueLabel(atlas, axis.id, v.id)}</span>
                    <span className="text-[11px] text-muted-foreground tabular-nums">
                      {formatCount(x?.weight ?? 0)} / {formatCount(x?.around ?? 0)}
                    </span>
                  </button>
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </div>
  );
}
