import { useEffect, useId, useMemo, useState } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import { cn } from "@neokapi/ui-primitives";
import { geoContains, geoPath } from "d3-geo";
import type { Feature, FeatureCollection, MultiPolygon, Polygon } from "geojson";
import { BORDER, HATCH_STRIPE, LAND } from "../colors";
import { type ShapeSet, countryOfSubdivision, loadCountries, loadSubdivisions } from "../geo";
import { SPHERE, around, fittedProjection, isPolygon, mainland, rewound } from "../geo/shape";
import { placeName } from "../labels";
import { WORLD, commonPlace, mapLevel, placeCounts } from "../model/places";
import { useElementWidth } from "./hooks";
import { PlacePane, type PlaceRow } from "./PlacePane";
import {
  type MapScope,
  isLit,
  isRetained,
  onActivate,
  pickHandlers,
  picksFor,
  placeFill,
  placeLabel,
  recordPlaces,
} from "./mapShared";

type Shape = Polygon | MultiPolygon;

interface Loaded {
  world?: ShapeSet;
  subs: Map<string, ShapeSet | null>;
  failed: boolean;
}

/** Load the world and the named countries' subdivisions, keeping what earlier renders loaded. */
function useShapes(countries: readonly string[]) {
  const key = [...new Set(countries)].filter(Boolean).sort().join(",");
  const [state, setState] = useState<Loaded>({ subs: new Map(), failed: false });
  useEffect(() => {
    let live = true;
    const ccs = key ? key.split(",") : [];
    Promise.all([
      loadCountries(),
      Promise.all(ccs.map((cc) => loadSubdivisions(cc).then((s) => [cc, s] as const))),
    ])
      .then(([world, subs]) => {
        if (!live) return;
        setState((prev) => {
          const next = new Map(prev.subs);
          for (const [cc, s] of subs) next.set(cc, s);
          return { world, subs: next, failed: false };
        });
      })
      .catch(() => {
        if (live) setState((prev) => ({ ...prev, failed: true }));
      });
    return () => {
      live = false;
    };
  }, [key]);
  const ready = !!state.world && (key === "" || key.split(",").every((cc) => state.subs.has(cc)));
  return { ...state, ready };
}

function asFeature(geometry: Shape): Feature<Shape> {
  return { type: "Feature", properties: {}, geometry };
}

export interface ShapeMapProps {
  scope: MapScope;
  /** The place the map opens at; by default the deepest country or region holding every record. */
  initialFocus?: string;
}

/**
 * The shaped map. It draws the world by country, a country by subdivision,
 * and a subdivision or a declared place by the places inside it, with a path
 * back up. A place is filled by the records sitting within it and hatched
 * where a record at a wider place, or at one sharing ground with it, holds.
 * A declared place draws its own shape, or a circle at its centre, or is
 * listed beside the map. Places made of others are chips beside the map;
 * hovering one outlines what it covers.
 */
export function ShapeMap({ scope, initialFocus }: ShapeMapProps) {
  const { atlas } = scope;
  const index = atlas.places;
  const [focus, setFocus] = useState(() => {
    if (initialFocus && index.has(initialFocus)) return initialFocus;
    const weighed = [...scope.byPlace.entries()].filter(([, x]) => x.weight > 0).map(([id]) => id);
    return commonPlace(index, weighed.length > 0 ? weighed : recordPlaces(scope));
  });
  const [highlight, setHighlight] = useState<string | null>(null);
  const level = useMemo(() => mapLevel(index, focus), [index, focus]);
  const counts = useMemo(
    () => placeCounts(index, level.drawn, scope.byPlace),
    [index, level, scope.byPlace],
  );

  const needed = useMemo(() => {
    const out = new Set<string>();
    const add = (id: string) => {
      const type = index.get(id)?.type;
      if (type === "country") out.add(id);
      else if (type === "subdivision") out.add(countryOfSubdivision(id));
    };
    if (level.layer === "inside") {
      for (const id of index.path(focus)) add(id);
      for (const d of level.drawn) {
        add(d);
        for (const w of index.get(d)?.within ?? []) for (const a of index.path(w)) add(a);
      }
    }
    return [...out];
  }, [index, level, focus]);
  const shapes = useShapes(needed);

  // A shape for a place: a declared place's own geometry, a subdivision's
  // Natural Earth outline, a country's outline (from its subdivisions when
  // they are loaded and `fine` is asked, else the world file's).
  const shapeOf = useMemo(() => {
    const declared = new Map<string, Shape>();
    for (const p of index.places)
      if (isPolygon(p.geometry)) declared.set(p.id, rewound(p.geometry));
    return (id: string, fine = false): Shape | undefined => {
      const own = declared.get(id);
      if (own) return own;
      const type = index.get(id)?.type;
      if (type === "country") {
        return (
          (fine ? shapes.subs.get(id)?.outline?.geometry : undefined) ??
          shapes.world?.get(id)?.geometry
        );
      }
      if (type === "subdivision") {
        return shapes.subs.get(countryOfSubdivision(id))?.get(id)?.geometry;
      }
      return undefined;
    };
  }, [index, shapes]);

  const anchor = useMemo(() => {
    if (level.layer === "countries") return undefined;
    const path = index.path(focus);
    for (let k = path.length - 1; k >= 0; k--) {
      const g = shapeOf(path[k], true);
      if (g) return { id: path[k], shape: g };
    }
    return undefined;
  }, [index, focus, level, shapeOf]);

  const [ref, width] = useElementWidth<HTMLDivElement>(640);
  const height = Math.round(Math.min(520, Math.max(260, width * (focus === WORLD ? 0.52 : 0.66))));

  const target = useMemo((): FeatureCollection | typeof SPHERE => {
    if (!shapes.ready || focus === WORLD) return SPHERE;
    const features: Feature[] = [];
    const push = (g: Shape | undefined) => {
      if (g) features.push(asFeature(mainland(g)));
    };
    if (level.layer === "countries") {
      for (const d of level.drawn) push(shapeOf(d));
    } else {
      const own = shapeOf(focus, true);
      if (own) push(own);
      else {
        for (const d of level.drawn) {
          const g = shapeOf(d);
          const c = index.get(d)?.centroid;
          if (g) push(g);
          else if (c) features.push(asFeature(around(c)));
        }
        const c = index.get(focus)?.centroid;
        if (c) features.push(asFeature(around(c, 0.08)));
        if (features.length === 0) push(anchor?.shape);
      }
    }
    return features.length > 0 ? { type: "FeatureCollection", features } : SPHERE;
  }, [shapes.ready, focus, level, shapeOf, index, anchor]);

  const projection = useMemo(
    () => fittedProjection(target, width, height),
    [target, width, height],
  );
  const path = useMemo(() => geoPath(projection), [projection]);

  const outsideOf = (d: string): string | undefined => {
    const p = index.get(d);
    if (!p?.declared || !p.centroid) return undefined;
    const point: [number, number] = [p.centroid[1], p.centroid[0]];
    for (const w of p.within ?? []) {
      const up = index.path(w);
      for (let k = up.length - 1; k >= 0; k--) {
        if (up[k] === WORLD) break;
        const g = shapeOf(up[k], true);
        if (!g) continue;
        if (!geoContains(g, point)) return up[k];
        break;
      }
    }
    return undefined;
  };

  const rows: PlaceRow[] = level.drawn.map((id) => ({
    id,
    count: counts.get(id),
    placed: !shapes.ready || !!shapeOf(id) || !!index.get(id)?.centroid,
    outside: shapes.ready ? outsideOf(id) : undefined,
    drillable: index.children(id).length > 0,
  }));

  const max = Math.max(1, ...level.drawn.map((d) => counts.get(d)?.sits.weight ?? 0));
  const lit = new Set(highlight ? level.drawn.filter((d) => index.overlaps(highlight, d)) : []);
  const hatchId = `atlas-map-hatch-${useId().replace(/[^a-zA-Z0-9-]/gu, "")}`;
  const drawnSet = new Set(level.drawn);
  const crumbs = index.path(focus);
  const drill = (id: string) => {
    if (index.has(id)) {
      setFocus(id);
      setHighlight(null);
      scope.onPreview(null);
    }
  };

  const mark = (id: string) => {
    const c = counts.get(id);
    const picks = picksFor(scope, id);
    const h = pickHandlers(scope, picks);
    const retained = isRetained(scope, picks, id);
    const held = (c?.held.weight ?? 0) > 0;
    const dim = !isLit(scope, id);
    const label = placeLabel(scope, id, c);
    const common = {
      role: "button",
      tabIndex: 0,
      "aria-label": label,
      "aria-pressed": retained,
      "data-testid": `atlas-map-place-${id}`,
      className: cn("cursor-pointer outline-none focus-visible:stroke-ring", dim && "opacity-35"),
      onMouseEnter: h.onMouseEnter,
      onMouseLeave: h.onMouseLeave,
      onFocus: h.onFocus,
      onBlur: h.onBlur,
      onClick: h.onClick,
      onDoubleClick: () => index.children(id).length > 0 && drill(id),
      onKeyDown: onActivate(h.onClick),
      fill: placeFill(scope, c, max),
      stroke: retained || lit.has(id) ? "var(--foreground)" : BORDER,
      strokeWidth: retained || lit.has(id) ? 2 : 0.6,
    } as const;
    const g = shapeOf(id);
    if (g) {
      const d = path(g) ?? "";
      return (
        <g key={id}>
          <path {...common} d={d}>
            <title>{label}</title>
          </path>
          {held && (
            // The hatch carries the outline too, so neighbouring places
            // under one hatch keep their borders.
            <path
              d={d}
              fill={`url(#${hatchId})`}
              stroke={common.stroke}
              strokeWidth={common.strokeWidth}
              pointerEvents="none"
              aria-hidden
            />
          )}
        </g>
      );
    }
    // A built-in place too small for Natural Earth to draw takes a circle only
    // when records sit within it; a declared place always does.
    const place = index.get(id);
    if (!place?.declared && (c?.sits.around ?? 0) === 0) return null;
    const centroid = place?.centroid;
    const xy = centroid ? projection([centroid[1], centroid[0]]) : null;
    if (!xy) return null;
    return (
      <g key={id}>
        <circle {...common} cx={xy[0]} cy={xy[1]} r={7}>
          <title>{label}</title>
        </circle>
        {held && (
          <circle
            cx={xy[0]}
            cy={xy[1]}
            r={7}
            fill={`url(#${hatchId})`}
            pointerEvents="none"
            aria-hidden
          />
        )}
      </g>
    );
  };

  return (
    <div className="flex flex-col gap-2">
      <nav aria-label={t("Place path", "map breadcrumb")} data-testid="atlas-map-path">
        <ol className="flex flex-wrap items-center gap-1 text-sm">
          {crumbs.map((id, k) => (
            <li key={id} className="flex items-center gap-1">
              {k > 0 && (
                <span aria-hidden className="text-muted-foreground">
                  ›
                </span>
              )}
              {k === crumbs.length - 1 ? (
                <span aria-current="page" className="font-medium">
                  {placeName(atlas, id)}
                </span>
              ) : (
                <button
                  type="button"
                  className="rounded px-1 text-muted-foreground hover:bg-muted hover:text-foreground"
                  onClick={() => drill(id)}
                >
                  {placeName(atlas, id)}
                </button>
              )}
            </li>
          ))}
        </ol>
      </nav>
      <div className="flex flex-col items-start gap-6 md:flex-row">
        <div ref={ref} className="w-full min-w-0 flex-1">
          <svg
            width={width}
            height={height}
            viewBox={`0 0 ${width} ${height}`}
            role="group"
            aria-label={t("Map of {place}", "map chart", { place: placeName(atlas, focus) })}
            aria-busy={!shapes.ready}
            data-testid="atlas-map-shapes"
            className="block max-w-full"
          >
            <defs>
              <pattern
                id={hatchId}
                width="6"
                height="6"
                patternUnits="userSpaceOnUse"
                patternTransform="rotate(45)"
              >
                <rect width="2.4" height="6" fill={HATCH_STRIPE} />
              </pattern>
            </defs>
            {shapes.ready && (
              <>
                <g aria-hidden>
                  {(shapes.world?.features ?? []).map((f, k) =>
                    level.layer === "countries" &&
                    f.id !== undefined &&
                    drawnSet.has(String(f.id)) ? null : (
                      <path
                        key={f.id === undefined ? `land-${k}` : String(f.id)}
                        d={path(f) ?? ""}
                        fill={
                          level.layer === "countries"
                            ? LAND
                            : "color-mix(in oklab, var(--atlas-land) 45%, var(--background))"
                        }
                        stroke={BORDER}
                        strokeWidth={0.4}
                      />
                    ),
                  )}
                  {anchor && (
                    <path
                      d={path(anchor.shape) ?? ""}
                      fill={LAND}
                      stroke={BORDER}
                      strokeWidth={1}
                    />
                  )}
                </g>
                <g>{level.drawn.map(mark)}</g>
              </>
            )}
            {!shapes.ready && (
              <text x={16} y={28} fill="var(--muted-foreground)" fontSize={12}>
                {shapes.failed
                  ? t("The shapes could not be loaded", "map: shapes failed to load")
                  : t("Loading shapes", "map: shapes loading")}
              </text>
            )}
          </svg>
          <ul
            className="mt-2 flex flex-wrap items-center gap-4 text-[11px] text-muted-foreground"
            aria-label={t("Legend", "map legend")}
          >
            <li className="flex items-center gap-1.5">
              <span
                aria-hidden
                className="inline-block size-3 rounded-sm"
                style={{
                  background: "color-mix(in oklab, var(--atlas-seq) 60%, var(--atlas-land))",
                }}
              />
              {t("Records within", "map legend: fill")}
            </li>
            <li className="flex items-center gap-1.5">
              <svg aria-hidden width="12" height="12">
                <rect width="12" height="12" fill={LAND} />
                <rect width="12" height="12" fill={`url(#${hatchId})`} />
              </svg>
              {t("Held through a wider place", "map legend: hatch")}
            </li>
            <li className="flex items-center gap-1.5">
              <span aria-hidden className="inline-block size-3 rounded-full border border-border" />
              {t("Drawn at its centre", "map legend: circle")}
            </li>
          </ul>
        </div>
        <PlacePane
          scope={scope}
          wider={level.wider}
          chips={level.chips}
          rows={rows}
          onDrill={drill}
          onHighlight={setHighlight}
        />
      </div>
    </div>
  );
}
