// The built-in shapes: Natural Earth countries and subdivisions, keyed by
// place id (`no`, `no-03`). Each file is imported on first use, so a bundler
// puts the world and every country's subdivisions in chunks of their own and
// the map reads only what it draws. See README.md for the sources.

import { feature } from "topojson-client";
import type { GeometryCollection, Topology } from "topojson-specification";
import type { Feature, MultiPolygon, Polygon } from "geojson";

/** A built-in shape: a polygon or multipolygon with an English name. */
export type PlaceFeature = Feature<Polygon | MultiPolygon, { name?: string }>;

/** The shapes in one file, by place id. */
export interface ShapeSet {
  /** Every shape in file order; a shape with no place id (`id` undefined) is land with nothing behind it. */
  features: PlaceFeature[];
  get(id: string): PlaceFeature | undefined;
  has(id: string): boolean;
  /** A country file's outline: the union of its subdivisions. */
  outline?: PlaceFeature;
}

type Loader = () => Promise<unknown>;

const WORLD = import.meta.glob("./world.json") as Record<string, Loader>;
const ADMIN1 = import.meta.glob("./admin1/*.json") as Record<string, Loader>;

function topologyOf(mod: unknown): Topology {
  const m = mod as { default?: Topology };
  return (m.default ?? mod) as Topology;
}

function shapes(topo: Topology, object: string): PlaceFeature[] {
  const o = topo.objects[object] as GeometryCollection | undefined;
  if (!o) return [];
  return feature(topo, o).features.filter(
    (f): f is PlaceFeature =>
      !!f.geometry && (f.geometry.type === "Polygon" || f.geometry.type === "MultiPolygon"),
  );
}

function shapeSet(features: PlaceFeature[], outline?: PlaceFeature): ShapeSet {
  const byId = new Map<string, PlaceFeature>();
  for (const f of features) if (f.id !== undefined) byId.set(String(f.id), f);
  return {
    features,
    get: (id) => byId.get(id),
    has: (id) => byId.has(id),
    outline,
  };
}

let world: Promise<ShapeSet> | undefined;
const countries = new Map<string, Promise<ShapeSet | null>>();

/** The world's countries. */
export function loadCountries(): Promise<ShapeSet> {
  const load = WORLD["./world.json"];
  world ??= load().then((m) => shapeSet(shapes(topologyOf(m), "countries")));
  return world;
}

/** One country's subdivisions and outline, by the country's place id; null when none ship. */
export function loadSubdivisions(country: string): Promise<ShapeSet | null> {
  const cc = country.toLowerCase();
  let p = countries.get(cc);
  if (!p) {
    const load = /^[a-z]{2}$/u.test(cc) ? ADMIN1[`./admin1/${cc}.json`] : undefined;
    p = load
      ? load().then((m) => {
          const topo = topologyOf(m);
          return shapeSet(shapes(topo, "subdivisions"), shapes(topo, "outline")[0]);
        })
      : Promise.resolve(null);
    countries.set(cc, p);
  }
  return p;
}

/** The country a built-in subdivision id sits in: `no-03` → `no`; empty for anything else. */
export function countryOfSubdivision(id: string): string {
  const m = /^([a-z]{2})-[a-z0-9]{1,3}$/u.exec(id);
  return m ? m[1] : "";
}
