import { useMemo, useState } from "react";
import { geoNaturalEarth1, geoPath } from "d3-geo";
import { feature } from "topojson-client";
import type { Topology, GeometryCollection } from "topojson-specification";
import countriesTopo from "world-atlas/countries-110m.json";
import { cn, resolveLocaleName } from "@neokapi/ui";
import { formatSessions, formatShare, type CountryDemand } from "./locale-demand-fixtures";

// react-simple-maps peer-locks React ≤18, so this renders the world-atlas
// TopoJSON directly: topojson-client → GeoJSON features, d3-geo
// (geoNaturalEarth1) → SVG paths. Plain SVG keeps hover/click/theming fully
// under our control.

const MAP_WIDTH = 960;
const MAP_HEIGHT = 500;

interface CountryFeature {
  id: string;
  name: string;
  d: string;
  centroid: [number, number];
}

type CountriesTopology = Topology<{
  countries: GeometryCollection<{ name: string }>;
}>;

let cachedFeatures: CountryFeature[] | null = null;

/** Project the world-atlas countries once — the geometry never changes. */
function worldFeatures(): CountryFeature[] {
  if (cachedFeatures) return cachedFeatures;
  const topology = countriesTopo as unknown as CountriesTopology;
  const projection = geoNaturalEarth1().fitExtent(
    [
      [4, 4],
      [MAP_WIDTH - 4, MAP_HEIGHT - 4],
    ],
    { type: "Sphere" },
  );
  const path = geoPath(projection);
  const collection = feature(topology, topology.objects.countries);
  cachedFeatures = collection.features
    .map((f) => ({
      id: String(f.id ?? ""),
      name: f.properties?.name ?? "",
      d: path(f) ?? "",
      centroid: path.centroid(f) as [number, number],
    }))
    .filter((f) => f.d !== "");
  return cachedFeatures;
}

export interface WorldDemandMapProps {
  countries: CountryDemand[];
  /** ISO alpha-2 of the selected (drilled-down) country. */
  selectedCountry?: string | null;
  onSelectCountry?: (code: string | null) => void;
  /** Pre-set hover state — used by Storybook/screenshots; live hover overrides it. */
  initialHoveredCountry?: string | null;
  className?: string;
}

/** Demand hue — same chart color the rest of the app's analytics use. */
const DEMAND_COLOR = "var(--chart-1, oklch(0.646 0.222 41.116))";

/** Demand intensity 0..1 → themed choropleth fill. */
export function demandFill(intensity: number): string {
  // sqrt spreads the low end so mid-size markets stay distinguishable; the
  // 25% floor keeps the lowest demand tier visible against the card surface.
  const pct = Math.round((0.25 + 0.6 * Math.sqrt(intensity)) * 100);
  return `color-mix(in oklab, ${DEMAND_COLOR} ${pct}%, var(--card))`;
}

export function WorldDemandMap({
  countries,
  selectedCountry,
  onSelectCountry,
  initialHoveredCountry = null,
  className,
}: WorldDemandMapProps) {
  const [hovered, setHovered] = useState<string | null>(initialHoveredCountry);

  const features = useMemo(worldFeatures, []);
  const byNumericId = useMemo(() => {
    const map = new Map<string, CountryDemand>();
    for (const c of countries) map.set(c.numericId, c);
    return map;
  }, [countries]);
  const maxShare = useMemo(
    () => countries.reduce((max, c) => Math.max(max, c.share), 0) || 1,
    [countries],
  );

  const hoveredFeature = hovered ? features.find((f) => f.id === hovered) : undefined;
  const hoveredCountry = hovered ? byNumericId.get(hovered) : undefined;

  // Tooltip anchors to the country centroid (stable, no cursor jitter).
  const tooltipLeft = hoveredFeature ? (hoveredFeature.centroid[0] / MAP_WIDTH) * 100 : 0;
  const tooltipTop = hoveredFeature ? (hoveredFeature.centroid[1] / MAP_HEIGHT) * 100 : 0;

  return (
    <div className={cn("relative", className)} data-testid="world-demand-map">
      <svg
        viewBox={`0 0 ${MAP_WIDTH} ${MAP_HEIGHT}`}
        className="w-full h-auto select-none"
        role="img"
        aria-label="World map of end-user language demand"
        onMouseLeave={() => setHovered(null)}
      >
        {features.map((f) => {
          const demand = byNumericId.get(f.id);
          const isSelected = !!demand && demand.code === selectedCountry;
          const isHovered = f.id === hovered;
          return (
            <path
              key={`${f.id}:${f.name}`}
              d={f.d}
              data-country={demand?.code}
              fill={demand ? demandFill(demand.share / maxShare) : "var(--muted)"}
              fillOpacity={demand ? 1 : 0.35}
              stroke={isSelected ? "var(--ring)" : "var(--border)"}
              strokeWidth={isSelected ? 1.75 : 0.5}
              className={cn(demand && "cursor-pointer", isHovered && "brightness-110")}
              onMouseEnter={() => setHovered(f.id)}
              onClick={() => {
                if (!demand || !onSelectCountry) return;
                onSelectCountry(isSelected ? null : demand.code);
              }}
            >
              {!demand && <title>{`${f.name}: no demand data`}</title>}
            </path>
          );
        })}
      </svg>

      {hoveredFeature && (
        <div
          className="pointer-events-none absolute z-10 w-56 -translate-x-1/2 rounded-lg border border-border/50 bg-background px-3 py-2 text-xs shadow-xl"
          style={{ left: `${tooltipLeft}%`, top: `calc(${tooltipTop}% + 12px)` }}
          data-testid="map-tooltip"
        >
          {hoveredCountry ? (
            <>
              <div className="flex items-center justify-between gap-2 font-medium">
                <span>
                  {hoveredCountry.flag} {hoveredCountry.name}
                </span>
                <span className="text-muted-foreground">
                  {formatSessions(hoveredCountry.sessions)} sessions
                </span>
              </div>
              <div className="mt-1 space-y-0.5 text-muted-foreground">
                {hoveredCountry.languages.slice(0, 3).map((l) => (
                  <div key={l.code} className="flex items-center justify-between">
                    <span>{resolveLocaleName(l.code)}</span>
                    <span>{formatShare(l.share)}</span>
                  </div>
                ))}
              </div>
              <div
                className="mt-1 border-t border-border/50 pt-1 text-muted-foreground"
                title={
                  hoveredCountry.servedRate === null ? "Not derivable from this source" : undefined
                }
              >
                Served their language:{" "}
                {hoveredCountry.servedRate === null ? "·" : formatShare(hoveredCountry.servedRate)}
              </div>
            </>
          ) : (
            <div className="text-muted-foreground">{hoveredFeature.name}: no demand data</div>
          )}
        </div>
      )}

      <MapLegend />
    </div>
  );
}

function MapLegend() {
  return (
    <div className="mt-2 flex items-center gap-3 text-xs text-muted-foreground">
      <span>Demand share</span>
      <div
        className="h-2 w-32 rounded-full"
        style={{
          background: `linear-gradient(to right, ${demandFill(0)}, ${demandFill(0.5)}, ${demandFill(1)})`,
        }}
      />
      <span>low → high</span>
      <span className="ml-4 inline-flex items-center gap-1.5">
        <span className="inline-block size-2.5 rounded-sm bg-muted/60" />
        no data
      </span>
    </div>
  );
}
