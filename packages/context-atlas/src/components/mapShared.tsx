// What the shaped map and the tile grid share: the counts per place, the
// picks a place retains, the colour of a count and the accessible name.

import type { KeyboardEvent } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import { attentionTint, sequentialFill } from "../colors";
import { formatCount, formatShare, placeName } from "../labels";
import type { Atlas, AxisInfo } from "../model/atlas";
import type { Pick } from "../model/filter";
import { type PlaceCount, type PlaceTally, overlapping } from "../model/places";
import { type AtlasQuery, withRetained } from "../model/query";

export type MapMeasure = "count" | "attention";

/** Everything a map surface reads, gathered once by MapView. */
export interface MapScope {
  atlas: Atlas;
  axis: AxisInfo;
  query: AtlasQuery;
  measure: MapMeasure;
  /** Tallies by the place a record sits at. */
  byPlace: ReadonlyMap<string, PlaceTally>;
  /** Weight per place among the records a hover previews; null without a hover. */
  previewCounts: ReadonlyMap<string, number> | null;
  onPreview: (picks: Pick[] | null) => void;
  onQuery: (next: AtlasQuery) => void;
}

/** The places records sit at on the axis, in the surrounding cohort. */
export function recordPlaces(scope: MapScope): string[] {
  return [...scope.byPlace.entries()]
    .filter(([id, x]) => id !== "" && (x.around > 0 || x.weight > 0))
    .map(([id]) => id);
}

/** What selecting a place retains: every record at a place that shares ground with it. */
export function picksFor(scope: MapScope, id: string): Pick[] {
  return overlapping(scope.atlas.places, id, recordPlaces(scope)).map((value) => ({
    axis: scope.axis.id,
    value,
  }));
}

/**
 * Whether the query retains every pick. With `place`, the query must also
 * keep a value inside that place: selecting Greater Oslo retains the records
 * at Norway, and Rogaland, which holds only those, does not read as selected.
 */
export function isRetained(scope: MapScope, picks: readonly Pick[], place?: string): boolean {
  const kept = scope.query.filters[scope.axis.id];
  if (picks.length === 0 || !kept || !picks.every((p) => kept.includes(p.value))) return false;
  return place === undefined || kept.some((v) => scope.atlas.places.contains(place, v));
}

/** Whether a hover's preview reaches a place: some previewed record holds there. */
export function isLit(scope: MapScope, id: string): boolean {
  if (scope.previewCounts === null) return true;
  for (const [at, n] of scope.previewCounts) {
    if (n > 0 && scope.atlas.places.overlaps(at, id)) return true;
  }
  return false;
}

/** Hover, focus and click handlers that preview and retain `picks`. */
export function pickHandlers(scope: MapScope, picks: Pick[]) {
  return {
    onMouseEnter: () => onPreviewOrNull(scope, picks),
    onMouseLeave: () => scope.onPreview(null),
    onFocus: () => onPreviewOrNull(scope, picks),
    onBlur: () => scope.onPreview(null),
    onClick: (e: { shiftKey: boolean }) => {
      scope.onPreview(null);
      if (picks.length > 0) scope.onQuery(withRetained(scope.query, picks, e.shiftKey));
    },
  };
}

function onPreviewOrNull(scope: MapScope, picks: Pick[]) {
  scope.onPreview(picks.length > 0 ? picks : null);
}

/** The fill of a drawn place: the sequential scale by count, or the attention scale by share. */
export function placeFill(scope: MapScope, c: PlaceCount | undefined, max: number): string {
  const sits = c?.sits;
  if (!sits || sits.weight === 0) return sequentialFill(null);
  if (scope.measure === "attention") return attentionTint(sits.attention / sits.weight);
  return sequentialFill(sits.weight / Math.max(1, max));
}

/** A drawn place's accessible name: its count, its share needing a person, what holds there. */
export function placeLabel(scope: MapScope, id: string, c: PlaceCount | undefined): string {
  const sits = c?.sits;
  const own = t("{place}: {count} selected, {share} need a person", "map place", {
    place: placeName(scope.atlas, id),
    count: formatCount(sits?.weight ?? 0),
    share: formatShare(sits && sits.weight > 0 ? sits.attention / sits.weight : null),
  });
  if (!c || c.held.weight === 0) return own;
  return t(
    "{own}; {count} more hold here through {places}",
    "map place held through a wider place",
    {
      own,
      count: formatCount(c.held.weight),
      places: c.heldThrough.map((p) => placeName(scope.atlas, p)).join(", "),
    },
  );
}

/** Keyboard activation for an SVG mark that acts as a button. */
export function onActivate(handler: (e: { shiftKey: boolean }) => void) {
  return (e: KeyboardEvent) => {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      handler(e);
    }
  };
}
