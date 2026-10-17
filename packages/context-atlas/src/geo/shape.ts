// Geometry helpers for the map: winding a declared shape for d3, the part of
// a country a projection fits to, and the projection itself.

import {
  type GeoPermissibleObjects,
  type GeoProjection,
  geoArea,
  geoBounds,
  geoCentroid,
  geoCircle,
  geoConicConformal,
  geoDistance,
  geoNaturalEarth1,
} from "d3-geo";
import type { Feature, FeatureCollection, MultiPolygon, Polygon, Position } from "geojson";
import type { AtlasGeometry } from "../types";

type Rings = Position[][];

/**
 * d3-geo reads a spherical polygon's exterior ring clockwise. A shape written
 * the RFC 7946 way, counter-clockwise, would enclose the rest of the globe;
 * each polygon whose area exceeds a hemisphere is reversed.
 */
export function rewound(geometry: AtlasGeometry): Polygon | MultiPolygon {
  const fix = (rings: Rings): Rings =>
    geoArea({ type: "Polygon", coordinates: rings }) > 2 * Math.PI
      ? rings.map((ring) => [...ring].reverse())
      : rings;
  if (geometry.type === "Polygon")
    return { type: "Polygon", coordinates: fix(geometry.coordinates) };
  return { type: "MultiPolygon", coordinates: geometry.coordinates.map(fix) };
}

/** Whether a value read from the atlas is a polygon d3 can draw. */
export function isPolygon(g: unknown): g is AtlasGeometry {
  if (!g || typeof g !== "object") return false;
  const { type, coordinates } = g as { type?: unknown; coordinates?: unknown };
  return (type === "Polygon" || type === "MultiPolygon") && Array.isArray(coordinates);
}

/** How far from its largest part a smaller part of a country still counts for fitting: 12°. */
const NEAR = (12 * Math.PI) / 180;
/** A part at least this share of the largest counts wherever it lies. */
const LARGE = 0.3;

/**
 * The part of a shape a projection fits to: its largest polygon, those near
 * it, and those nearly as large. France fits to the mainland and Corsica,
 * not to its overseas departments; Norway to its mainland, not to Svalbard;
 * Indonesia to its main islands. The parts left out are still drawn.
 */
export function mainland(geometry: Polygon | MultiPolygon): Polygon | MultiPolygon {
  if (geometry.type === "Polygon" || geometry.coordinates.length < 2) return geometry;
  const parts = geometry.coordinates.map((coordinates) => {
    const p: Polygon = { type: "Polygon", coordinates };
    return { coordinates, area: geoArea(p), centre: geoCentroid(p) };
  });
  const largest = parts.reduce((a, b) => (b.area > a.area ? b : a));
  return {
    type: "MultiPolygon",
    coordinates: parts
      .filter(
        (p) => p.area >= largest.area * LARGE || geoDistance(p.centre, largest.centre) <= NEAR,
      )
      .map((p) => p.coordinates),
  };
}

/** A small circle around `[lat, lon]`, so a lone point still gives a projection an extent. */
export function around([lat, lon]: [number, number], radiusDeg = 0.04): Polygon {
  return geoCircle().center([lon, lat]).radius(radiusDeg)() as Polygon;
}

/** The whole sphere, for the world map. */
export const SPHERE = { type: "Sphere" } as const;

/**
 * A projection fitted to `target` inside a `width` by `height` box: the
 * Natural Earth projection for the whole world, a conformal conic centred on
 * the extent for anything smaller.
 */
export function fittedProjection(
  target: FeatureCollection | Feature | typeof SPHERE,
  width: number,
  height: number,
  pad = 12,
): GeoProjection {
  const extent: [[number, number], [number, number]] = [
    [pad, pad],
    [Math.max(pad + 1, width - pad), Math.max(pad + 1, height - pad)],
  ];
  const obj = target as GeoPermissibleObjects;
  if (target.type === "Sphere") return geoNaturalEarth1().fitExtent(extent, obj);
  const [[, south], [, north]] = geoBounds(obj);
  const [lon] = geoCentroid(obj);
  const span = north - south;
  // Standard parallels a sixth of the way in from each edge keep the scale
  // error small across the extent; an extent split by the equator falls back
  // to Mercator inside d3.
  const p1 = south + span / 6;
  const p2 = north - span / 6;
  return geoConicConformal()
    .rotate([-lon, 0])
    .parallels([p1, p2 === p1 ? p1 + 0.01 : p2])
    .fitExtent(extent, obj);
}
