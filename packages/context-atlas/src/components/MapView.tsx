import { useMemo, useState } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import { SimpleTooltip, ViewTab, ViewTabGroup } from "@neokapi/ui-primitives";
import { Info } from "lucide-react";
import { axisLabel } from "../labels";
import { type Atlas, geoAxes, preparedStateAt } from "../model/atlas";
import { type EvalContext, type Pick, cohort, valueOf } from "../model/filter";
import type { PlaceTally } from "../model/places";
import type { AtlasQuery } from "../model/query";
import { NEEDS_PERSON } from "../model/state";
import { AxisSelect } from "./AxisSelect";
import { ShapeMap } from "./ShapeMap";
import { TileGrid } from "./TileGrid";
import type { MapMeasure, MapScope } from "./mapShared";

export type MapMode = "shapes" | "grid";

export interface MapViewProps {
  atlas: Atlas;
  query: AtlasQuery;
  ctx: EvalContext;
  foreground: readonly number[];
  preview: ReadonlySet<number> | null;
  onPreview: (picks: Pick[] | null) => void;
  onQuery: (next: AtlasQuery) => void;
  /** The drawing the map opens with: shapes (the default) or the tile grid. */
  initialMode?: MapMode;
  /** The place the shaped map opens at. */
  initialFocus?: string;
}

/**
 * The map: the records of a geo axis over the places they sit at. By default
 * it draws real shapes and drills from the world down to the project's own
 * places; the grid draws one equal tile per country instead, for comparing
 * counts without land area weighing in.
 */
export function MapView({
  atlas,
  query,
  ctx,
  foreground,
  preview,
  onPreview,
  onQuery,
  initialMode = "shapes",
  initialFocus,
}: MapViewProps) {
  const axes = geoAxes(atlas);
  const [axisId, setAxisId] = useState(axes[0]?.id ?? "");
  const axis = axes.find((a) => a.id === axisId) ?? axes[0];
  const [measure, setMeasure] = useState<MapMeasure>("count");
  const [mode, setMode] = useState<MapMode>(initialMode);

  const byPlace = useMemo(() => {
    const map = new Map<string, PlaceTally>();
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
  const scope: MapScope = {
    atlas,
    axis,
    query,
    measure,
    byPlace,
    previewCounts,
    onPreview,
    onQuery,
  };

  return (
    <div className="flex flex-col gap-3" data-testid="atlas-map">
      <div className="flex flex-wrap items-center gap-2">
        {axes.length > 1 && (
          <AxisSelect
            atlas={atlas}
            value={axis.id}
            kind="geo"
            onChange={setAxisId}
            label={t("Map axis", "map axis picker")}
          />
        )}
        {axes.length === 1 && (
          <span className="text-sm font-medium">{axisLabel(atlas, axis.id)}</span>
        )}
        <ViewTabGroup aria-label={t("Map drawing", "map mode")}>
          <ViewTab active={mode === "shapes"} onClick={() => setMode("shapes")}>
            {t("Shapes", "map mode: real shapes")}
          </ViewTab>
          <ViewTab active={mode === "grid"} onClick={() => setMode("grid")}>
            {t("Grid", "map mode: one equal tile per country")}
          </ViewTab>
        </ViewTabGroup>
        <ViewTabGroup aria-label={t("Map colour", "map measure")}>
          <ViewTab active={measure === "count"} onClick={() => setMeasure("count")}>
            {t("Selected", "map colour: records selected")}
          </ViewTab>
          <ViewTab active={measure === "attention"} onClick={() => setMeasure("attention")}>
            {t("Needs a person", "map colour: share waiting for a person")}
          </ViewTab>
        </ViewTabGroup>
        <SimpleTooltip
          content={
            mode === "grid"
              ? t(
                  "Each country is one tile, set near where it lies. Tile area is not geographic area, so a large country counts the same as a small one.",
                  "map info: grid",
                )
              : t(
                  "Shapes come from Natural Earth. A place is filled by the records within it and hatched where a record at a wider place holds.",
                  "map info: shapes",
                )
          }
        >
          <button
            type="button"
            className="inline-flex size-6 items-center justify-center rounded-md text-muted-foreground hover:bg-muted"
            aria-label={t("About the map", "map info button")}
          >
            <Info aria-hidden className="size-4" />
          </button>
        </SimpleTooltip>
      </div>
      {mode === "shapes" ? (
        <ShapeMap key={axis.id} scope={scope} initialFocus={initialFocus} />
      ) : (
        <TileGrid scope={scope} />
      )}
    </div>
  );
}
