import { t } from "@neokapi/i18n-react/runtime";
import { SimpleTooltip, cn } from "@neokapi/ui-primitives";
import { ChevronRight, MapPinOff, TriangleAlert } from "lucide-react";
import { formatCount, placeName } from "../labels";
import { type PlaceCount, WORLD } from "../model/places";
import { type MapScope, isRetained, pickHandlers, picksFor, placeLabel } from "./mapShared";

/** The most drawn places the list shows in full. */
const LIST_ALL = 30;

/** One place the map draws, as the list beside it shows it. */
export interface PlaceRow {
  id: string;
  count: PlaceCount | undefined;
  /** false: the place has no shape and no centroid, so the map lists it instead of drawing it. */
  placed: boolean;
  /** The place whose outline the place's centre falls outside, when it does. */
  outside?: string;
  drillable: boolean;
}

export interface PlacePaneProps {
  scope: MapScope;
  /** Places containing the view; records at them hold across it. */
  wider: string[];
  /** Places made of others, and regions, sharing ground with the view. */
  chips: string[];
  rows: PlaceRow[];
  onDrill?: (id: string) => void;
  /** Outline the places a chip covers while it is hovered or focused. */
  onHighlight?: (id: string | null) => void;
}

/**
 * The list beside the map: what holds across the whole view (the world's
 * records as one count), the places made of others as chips, and the drawn
 * places: all of them in a view of a few, else those with records within
 * them, without a shape, or flagged. Each entry
 * previews on hover and retains on click, as the map does.
 */
export function PlacePane({ scope, wider, chips, rows, onDrill, onHighlight }: PlacePaneProps) {
  const { atlas, byPlace } = scope;
  const at = (id: string) => byPlace.get(id);
  const widerRows = wider.filter((id) => (at(id)?.around ?? 0) > 0);
  const within = (chip: string) => {
    let weight = 0;
    let around = 0;
    for (const [p, x] of byPlace) {
      if (atlas.places.contains(chip, p)) {
        weight += x.weight;
        around += x.around;
      }
    }
    return { weight, around };
  };
  // A chip is a place records sit at; a region that only contains such
  // places is left to the breadcrumb.
  const chipRows = chips
    .filter((id) => (at(id)?.around ?? 0) > 0)
    .map((id) => ({ id, ...within(id) }));
  // A view of a few places lists each of them, so a county can be picked by
  // name. A view of many (the world's countries) lists what sits somewhere,
  // and what the map cannot draw or flags; the rest show as hatch.
  const listed = rows
    .filter(
      (r) =>
        rows.length <= LIST_ALL ||
        !r.placed ||
        r.outside !== undefined ||
        (r.count?.sits.around ?? 0) > 0,
    )
    .sort(
      (a, b) =>
        (b.count?.sits.weight ?? 0) - (a.count?.sits.weight ?? 0) ||
        (b.count?.held.weight ?? 0) - (a.count?.held.weight ?? 0) ||
        placeName(atlas, a.id).localeCompare(placeName(atlas, b.id)),
    );

  const drill = (id: string, drillable: boolean) =>
    drillable && onDrill ? (
      <button
        type="button"
        className="inline-flex size-6 shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground"
        aria-label={t("Show the places in {place}", "map: drill into a place", {
          place: placeName(atlas, id),
        })}
        onClick={() => onDrill(id)}
      >
        <ChevronRight aria-hidden className="size-4" />
      </button>
    ) : null;

  return (
    <div
      className="flex w-full min-w-56 flex-col gap-4 text-sm md:w-64"
      data-testid="atlas-map-pane"
    >
      {widerRows.length > 0 && (
        <div className="flex flex-col gap-0.5">
          <div aria-hidden className="px-2 text-[11px] text-muted-foreground">
            {t("Across this map", "map pane: records at places that contain the view")}
          </div>
          <ul
            aria-label={t("Across this map", "map pane: records at places that contain the view")}
          >
            {widerRows.map((id) => {
              const picks = [{ axis: scope.axis.id, value: id }];
              const h = pickHandlers(scope, picks);
              return (
                <li key={id}>
                  <button
                    type="button"
                    aria-pressed={isRetained(scope, picks)}
                    className={cn(
                      "flex w-full items-center justify-between gap-3 rounded-md px-2 py-1 text-left hover:bg-muted",
                      isRetained(scope, picks) && "ring-1 ring-foreground",
                    )}
                    {...h}
                  >
                    <span className="truncate">
                      {id === WORLD
                        ? t("Held everywhere", "map pane: records at the world")
                        : placeName(atlas, id)}
                    </span>
                    <span className="font-mono text-xs tabular-nums">
                      {formatCount(at(id)?.weight ?? 0)}
                    </span>
                  </button>
                </li>
              );
            })}
          </ul>
        </div>
      )}
      {chipRows.length > 0 && (
        <ul
          className="flex flex-wrap gap-1.5"
          aria-label={t("Places spanning others", "map pane: places made of others, and regions")}
        >
          {chipRows.map((c) => {
            const picks = picksFor(scope, c.id);
            const h = pickHandlers(scope, picks);
            const retained = isRetained(scope, picks, c.id);
            return (
              <li
                key={c.id}
                className={cn(
                  "inline-flex items-center rounded-full border border-border pr-0.5",
                  retained && "border-foreground",
                )}
              >
                <button
                  type="button"
                  aria-pressed={retained}
                  className="inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 hover:bg-muted"
                  onMouseEnter={() => {
                    h.onMouseEnter();
                    onHighlight?.(c.id);
                  }}
                  onMouseLeave={() => {
                    h.onMouseLeave();
                    onHighlight?.(null);
                  }}
                  onFocus={() => {
                    h.onFocus();
                    onHighlight?.(c.id);
                  }}
                  onBlur={() => {
                    h.onBlur();
                    onHighlight?.(null);
                  }}
                  onClick={h.onClick}
                >
                  <span>{placeName(atlas, c.id)}</span>
                  <span className="font-mono text-xs tabular-nums text-muted-foreground">
                    {formatCount(c.weight)}
                  </span>
                </button>
                {drill(c.id, atlas.places.children(c.id).length > 0)}
              </li>
            );
          })}
        </ul>
      )}
      {listed.length > 0 && (
        <div className="flex flex-col gap-1">
          <div
            aria-hidden
            className="flex items-center gap-3 px-2 text-[11px] text-muted-foreground"
          >
            <span className="flex-1" />
            <span className="w-10 text-right">{t("Here", "map pane column: records within")}</span>
            <span className="w-10 text-right">
              {t("Held", "map pane column: records held through a wider place")}
            </span>
            {onDrill && <span className="w-6" />}
          </div>
          <ul aria-label={t("Places on this map", "map pane: drawn places")}>
            {listed.map((r) => {
              const picks = picksFor(scope, r.id);
              const h = pickHandlers(scope, picks);
              const retained = isRetained(scope, picks, r.id);
              const held = r.count?.held.weight ?? 0;
              const unplaced = t(
                "No shape or centre: listed, not drawn",
                "map place without a shape",
              );
              const outside =
                r.outside === undefined
                  ? undefined
                  : t(
                      "Its centre lies outside {place}",
                      "map place: centroid outside the place it sits within",
                      { place: placeName(atlas, r.outside) },
                    );
              const name = [
                placeLabel(scope, r.id, r.count),
                r.placed ? "" : unplaced,
                outside ?? "",
              ]
                .filter(Boolean)
                .join("; ");
              return (
                <li key={r.id} className="flex items-center gap-1">
                  <button
                    type="button"
                    aria-pressed={retained}
                    aria-label={name}
                    className={cn(
                      "flex min-w-0 flex-1 items-center gap-3 rounded-md px-2 py-1 text-left hover:bg-muted",
                      retained && "ring-1 ring-foreground",
                    )}
                    {...h}
                  >
                    <span className="flex min-w-0 flex-1 items-center gap-1.5">
                      <span className="truncate">{placeName(atlas, r.id)}</span>
                      {!r.placed && (
                        <SimpleTooltip content={unplaced}>
                          <MapPinOff
                            aria-hidden
                            className="size-3.5 shrink-0 text-muted-foreground"
                            data-testid={`atlas-map-unplaced-${r.id}`}
                          />
                        </SimpleTooltip>
                      )}
                      {outside !== undefined && (
                        <SimpleTooltip content={outside}>
                          <TriangleAlert
                            aria-hidden
                            className="size-3.5 shrink-0 text-warning-ink"
                            data-testid={`atlas-map-flag-${r.id}`}
                          />
                        </SimpleTooltip>
                      )}
                    </span>
                    <span className="w-10 text-right font-mono text-xs tabular-nums">
                      {formatCount(r.count?.sits.weight ?? 0)}
                    </span>
                    <span className="w-10 text-right font-mono text-xs tabular-nums text-muted-foreground">
                      {held > 0
                        ? t("+{count}", "map pane: records held through a wider place", {
                            count: formatCount(held),
                          })
                        : ""}
                    </span>
                  </button>
                  {onDrill && (drill(r.id, r.drillable) ?? <span className="w-6 shrink-0" />)}
                </li>
              );
            })}
          </ul>
        </div>
      )}
    </div>
  );
}
