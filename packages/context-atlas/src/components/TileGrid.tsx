import { useId, useMemo } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import { cn } from "@neokapi/ui-primitives";
import { HATCH_STRIPE } from "../colors";
import { formatCount } from "../labels";
import { WORLD, mapLevel, placeCounts } from "../model/places";
import { gridPlaces, layoutTiles } from "../model/tiles";
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
} from "./mapShared";

const CELL = 46;
const TILE = 42;

/** Keeps a label legible over the hatched fill of a tile held through a wider place. */
const HALO = {
  stroke: "var(--background)",
  strokeWidth: 3,
  strokeLinejoin: "round",
  paintOrder: "stroke",
} as const;

/**
 * The tile grid: one square tile per country, set near where it lies. Tile
 * size is the same for every country, so land area never inflates a count.
 * A record at a place inside a country counts on the country's tile; a tile
 * is hatched where a record at a wider place holds.
 */
export function TileGrid({ scope }: { scope: MapScope }) {
  const index = scope.atlas.places;
  // A tile for each country where records sit or hold, in the surrounding
  // cohort, so a filter dims tiles rather than moving them.
  const { tiles, counts } = useMemo(() => {
    const all = gridPlaces(index);
    const c = placeCounts(
      index,
      all.map((p) => p.id),
      scope.byPlace,
    );
    const kept = all.filter((p) => {
      const x = c.get(p.id);
      return (x?.sits.around ?? 0) > 0 || (x?.held.around ?? 0) > 0;
    });
    return { tiles: kept, counts: c };
  }, [index, scope.byPlace]);
  const grid = useMemo(() => layoutTiles(tiles), [tiles]);
  const ids = useMemo(() => tiles.map((p) => p.id), [tiles]);
  const level = useMemo(() => mapLevel(index, WORLD), [index]);
  const hatchId = `atlas-grid-hatch-${useId().replace(/[^a-zA-Z0-9-]/gu, "")}`;
  const max = Math.max(1, ...ids.map((d) => counts.get(d)?.sits.weight ?? 0));
  const rows: PlaceRow[] = ids.map((id) => ({
    id,
    count: counts.get(id),
    placed: true,
    drillable: false,
  }));

  return (
    <div className="flex flex-col items-start gap-6 md:flex-row" data-testid="atlas-map-grid">
      <div className="overflow-x-auto">
        <svg
          width={grid.cols * CELL}
          height={grid.rows * CELL}
          role="group"
          aria-label={t("Country tiles", "map grid chart")}
        >
          <defs>
            <pattern
              id={hatchId}
              width="6"
              height="6"
              patternUnits="userSpaceOnUse"
              patternTransform="rotate(45)"
            >
              <rect width="2.5" height="6" fill={HATCH_STRIPE} />
            </pattern>
          </defs>
          {grid.tiles.map((tile) => {
            const c = counts.get(tile.id);
            const picks = picksFor(scope, tile.id);
            const h = pickHandlers(scope, picks);
            const retained = isRetained(scope, picks, tile.id);
            const sits = c?.sits.weight ?? 0;
            const held = c?.held.weight ?? 0;
            const label = placeLabel(scope, tile.id, c);
            // Over the hatch the label takes the foreground ink on a halo of
            // the background, so it reads on either stripe.
            const ink =
              held === 0 && sits > 0 && scope.measure === "count" && sits / max > 0.45
                ? "var(--primary-foreground)"
                : "var(--foreground)";
            return (
              <g
                key={tile.id}
                role="button"
                tabIndex={0}
                aria-label={label}
                aria-pressed={retained}
                data-testid={`atlas-map-tile-${tile.id}`}
                className={cn(
                  "cursor-pointer outline-none [&:focus-visible>rect]:stroke-ring",
                  !isLit(scope, tile.id) && "opacity-35",
                )}
                transform={`translate(${tile.col * CELL} ${tile.row * CELL})`}
                onMouseEnter={h.onMouseEnter}
                onMouseLeave={h.onMouseLeave}
                onFocus={h.onFocus}
                onBlur={h.onBlur}
                onClick={h.onClick}
                onKeyDown={onActivate(h.onClick)}
              >
                <title>{label}</title>
                <rect
                  width={TILE}
                  height={TILE}
                  rx={6}
                  fill={placeFill(scope, c, max)}
                  stroke={retained ? "var(--foreground)" : "var(--border)"}
                  strokeWidth={retained ? 2 : 1}
                  strokeDasharray={sits > 0 || held > 0 ? undefined : "3 3"}
                />
                {held > 0 && (
                  <rect
                    width={TILE}
                    height={TILE}
                    rx={6}
                    fill={`url(#${hatchId})`}
                    pointerEvents="none"
                  />
                )}
                <text
                  x={TILE / 2}
                  y={17}
                  textAnchor="middle"
                  fontSize={12}
                  fontWeight={600}
                  fill={ink}
                  {...(held > 0 ? HALO : {})}
                >
                  {tile.id.toUpperCase()}
                </text>
                <text
                  x={TILE / 2}
                  y={32}
                  textAnchor="middle"
                  fontSize={10}
                  fill={ink}
                  className="font-mono tabular-nums"
                  {...(held > 0 ? HALO : {})}
                >
                  {sits === 0 && held > 0
                    ? t("+{count}", "map tile: records holding through a wider place", {
                        count: formatCount(held),
                      })
                    : formatCount(sits)}
                </text>
              </g>
            );
          })}
        </svg>
      </div>
      <PlacePane scope={scope} wider={level.wider} chips={level.chips} rows={rows} />
    </div>
  );
}
