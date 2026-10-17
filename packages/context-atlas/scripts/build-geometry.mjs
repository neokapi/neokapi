#!/usr/bin/env node
// Builds the map's built-in shapes from Natural Earth (public domain):
//
//   src/geo/world.json          countries, keyed by ISO 3166-1 alpha-2 in lower case
//   src/geo/admin1/<cc>.json    one country's subdivisions, keyed by ISO 3166-2 in
//                               lower case (`no-03`), plus the country's outline
//
// Every file is TopoJSON, quantised and simplified. The map imports each one
// lazily, so a bundler splits them into chunks of their own.
//
// Usage: vp run build-geometry [-- --cache <dir>]
//
// The sources are fetched at a pinned release and checked against a SHA-256
// digest. With --cache (or NATURAL_EARTH_DIR), files already in that directory
// are read instead of downloaded.

import { createHash } from "node:crypto";
import { existsSync } from "node:fs";
import { mkdir, readFile, readdir, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { geoArea } from "d3-geo";
import { feature as toFeature, mergeArcs, quantize } from "topojson-client";
import { topology } from "topojson-server";
import { presimplify, sphericalTriangleArea } from "topojson-simplify";

export const NATURAL_EARTH_VERSION = "v5.1.2";

const SOURCES = {
  countries: {
    file: "ne_50m_admin_0_countries.geojson",
    sha256: "3e458fc036ad0a66411f2c1e6cac49c5d7bfb81cb1123bc513b22511a2b7fdeb",
  },
  subdivisions: {
    file: "ne_10m_admin_1_states_provinces.geojson",
    sha256: "22d0e3ad85eb3e27f17cabf8ba2d50e554fbc27a87796ff891d958185da62fb5",
  },
};

const BASE = `https://raw.githubusercontent.com/nvkelso/natural-earth-vector/${NATURAL_EARTH_VERSION}/geojson/`;

const HERE = dirname(fileURLToPath(import.meta.url));
const OUT = join(HERE, "..", "src", "geo");

/**
 * The detail kept. A point is dropped when the triangle it forms with its
 * neighbours is smaller than a share of the area of the smallest shape its
 * line borders, held between a floor and a cap (square kilometres). A small
 * county keeps the detail it needs to fill the map; the long coastline of a
 * large one does not swell the file. The world is drawn smaller than a
 * country, so it keeps less.
 */
const DETAIL = {
  world: { share: 2e-4, floor: 20, cap: 2000 },
  subdivisions: { share: 1e-4, floor: 0.2, cap: 400 },
};
/** Steradians per square kilometre on a sphere of the Earth's mean radius. */
const SR_PER_KM2 = 1 / 6371.0088 ** 2;
const QUANTISATION = 3e4;

const ISO2 = /^[A-Z]{2}$/u;
const ISO3166_2 = /^([A-Z]{2})-[A-Z0-9]{1,3}$/u;

function argValue(name) {
  const i = process.argv.indexOf(name);
  return i >= 0 ? process.argv[i + 1] : undefined;
}

async function source({ file, sha256 }) {
  const dir =
    argValue("--cache") ??
    process.env.NATURAL_EARTH_DIR ??
    join(tmpdir(), `natural-earth-${NATURAL_EARTH_VERSION}`);
  const path = join(dir, file);
  let bytes;
  if (existsSync(path)) {
    bytes = await readFile(path);
  } else {
    await mkdir(dir, { recursive: true });
    const res = await fetch(BASE + file);
    if (!res.ok) throw new Error(`${file}: HTTP ${res.status}`);
    bytes = Buffer.from(await res.arrayBuffer());
    await writeFile(path, bytes);
  }
  const digest = createHash("sha256").update(bytes).digest("hex");
  if (digest !== sha256) {
    throw new Error(
      `${file}: SHA-256 ${digest}, want ${sha256} (Natural Earth ${NATURAL_EARTH_VERSION})`,
    );
  }
  return JSON.parse(bytes.toString("utf8"));
}

/**
 * d3-geo reads a spherical polygon's exterior ring clockwise; a ring wound the
 * other way encloses the rest of the globe. Each polygon whose area exceeds a
 * hemisphere is reversed.
 */
function rewind(geometry) {
  const fix = (rings) => {
    if (geoArea({ type: "Polygon", coordinates: rings }) <= 2 * Math.PI) return rings;
    return rings.map((ring) => [...ring].reverse());
  };
  if (geometry.type === "Polygon") return { ...geometry, coordinates: fix(geometry.coordinates) };
  if (geometry.type === "MultiPolygon") {
    return { ...geometry, coordinates: geometry.coordinates.map(fix) };
  }
  return geometry;
}

function feature(id, name, geometry) {
  return { type: "Feature", id, properties: { name }, geometry: rewind(geometry) };
}

/** Quantise, simplify and merge the geometries that share an id into one. */
function build(name, features, detail, extra = () => ({})) {
  const topo = presimplify(
    topology({ [name]: { type: "FeatureCollection", features } }),
    sphericalTriangleArea,
  );
  simplifyBySize(topo, features, detail);

  const geoms = topo.objects[name].geometries;
  const byId = new Map();
  const merged = [];
  for (const g of geoms) {
    if (g.type === null) continue;
    if (g.id === undefined) {
      merged.push(g);
      continue;
    }
    const list = byId.get(g.id) ?? [];
    list.push(g);
    byId.set(g.id, list);
  }
  for (const [id, list] of byId) {
    if (list.length === 1) {
      merged.push(list[0]);
      continue;
    }
    const m = mergeArcs(topo, list);
    merged.push({ ...m, id, properties: list[0].properties });
  }
  merged.sort((a, b) => String(a.id ?? "~").localeCompare(String(b.id ?? "~")));
  topo.objects[name].geometries = merged;
  Object.assign(topo.objects, extra(topo, merged));
  return rewindTopology(quantize(topo, QUANTISATION));
}

/**
 * Simplifying and quantising can turn a small ring inside out. Each polygon
 * is checked again after both, and one that now encloses more than a
 * hemisphere has its rings reversed: the arc order flips and each arc index
 * becomes its complement.
 */
function rewindTopology(topo) {
  const flip = (rings) => rings.map((ring) => ring.map((i) => ~i).reverse());
  const fix = (rings) => {
    const f = toFeature(topo, { type: "Polygon", arcs: rings });
    return geoArea(f) > 2 * Math.PI ? flip(rings) : rings;
  };
  for (const object of Object.values(topo.objects)) {
    for (const g of object.geometries ?? [object]) {
      if (g.type === "Polygon") g.arcs = fix(g.arcs);
      else if (g.type === "MultiPolygon") g.arcs = g.arcs.map(fix);
    }
  }
  return topo;
}

/**
 * Drop each arc's points below the threshold of the smallest shape it
 * borders, and the weight presimplify stores beside each point.
 */
function simplifyBySize(topo, features, { share, floor, cap }) {
  const threshold = new Float64Array(topo.arcs.length).fill(Number.POSITIVE_INFINITY);
  const visit = (arcs, t) => {
    for (const a of arcs) {
      if (Array.isArray(a)) visit(a, t);
      else {
        const i = a < 0 ? ~a : a;
        if (t < threshold[i]) threshold[i] = t;
      }
    }
  };
  const [object] = Object.values(topo.objects);
  object.geometries.forEach((g, k) => {
    if (!g.arcs) return;
    const km2 = geoArea(features[k]) / SR_PER_KM2;
    visit(g.arcs, Math.min(cap, Math.max(floor, km2 * share)) * SR_PER_KM2);
  });
  topo.arcs = topo.arcs.map((arc, i) =>
    arc.filter((p) => p[2] >= threshold[i]).map((p) => [p[0], p[1]]),
  );
}

function countries(data) {
  const features = [];
  for (const f of data.features) {
    const p = f.properties;
    const code = [p.ISO_A2_EH, p.ISO_A2].find((c) => typeof c === "string" && ISO2.test(c));
    // A feature without an ISO code (Somaliland, Northern Cyprus) is drawn as
    // land with no place behind it.
    features.push(feature(code?.toLowerCase(), p.NAME_EN || p.NAME, f.geometry));
  }
  return build("countries", features, DETAIL.world);
}

/**
 * The successor table: current ISO 3166-2 ids Natural Earth carries under
 * another code or only as their parts, each composed from Natural Earth
 * features. A row that names a code Natural Earth lacks, or an id it already
 * carries, stops the build rather than drawing a wrong shape.
 */
async function successors() {
  const table = JSON.parse(await readFile(join(HERE, "geometry-successors.json"), "utf8"));
  const byCountry = new Map();
  for (const row of table.successors) {
    const cc = row.id.slice(0, 2);
    const list = byCountry.get(cc) ?? [];
    list.push(row);
    byCountry.set(cc, list);
  }
  return byCountry;
}

function subdivisions(data, successorRows) {
  const byCountry = new Map();
  for (const f of data.features) {
    const p = f.properties;
    const m = ISO3166_2.exec(p.iso_3166_2 ?? "");
    if (!m || !f.geometry) continue;
    const cc = m[1].toLowerCase();
    const list = byCountry.get(cc) ?? [];
    list.push(f);
    byCountry.set(cc, list);
  }
  const out = new Map();
  for (const [cc, list] of byCountry) {
    const own = new Set(list.map((f) => f.properties.iso_3166_2));
    const features = list.map((f) =>
      feature(
        f.properties.iso_3166_2.toLowerCase(),
        f.properties.name_en || f.properties.name,
        f.geometry,
      ),
    );
    // Where Natural Earth names an ISO 3166-2 region above a subdivision
    // (France's regions over its departments, Italy's over its provinces),
    // the region's shape is the union of its subdivisions.
    // Each composed shape, by id: its name and the ids of its parts.
    const composed = new Map();
    for (const f of list) {
      // Some codes and names carry stray whitespace in the source.
      const code = String(f.properties.region_cod ?? "").trim();
      if (own.has(code)) continue;
      const m = ISO3166_2.exec(code);
      if (!m || m[1].toLowerCase() !== cc) continue;
      const id = code.toLowerCase();
      const r = composed.get(id) ?? {
        name: String(f.properties.region ?? "").trim(),
        members: new Set(),
      };
      r.members.add(f.properties.iso_3166_2.toLowerCase());
      composed.set(id, r);
    }
    for (const row of successorRows.get(cc) ?? []) {
      if (own.has(row.id.toUpperCase()) || composed.has(row.id)) {
        throw new Error(
          `geometry-successors.json: ${row.id}: Natural Earth already carries this id`,
        );
      }
      const members = new Set();
      for (const code of row.from ?? []) {
        if (!own.has(code)) {
          throw new Error(`geometry-successors.json: ${row.id}: Natural Earth has no ${code}`);
        }
        members.add(code.toLowerCase());
      }
      for (const [field, value] of Object.entries(row.where ?? {})) {
        for (const f of list) {
          if (f.properties[field] === value) members.add(f.properties.iso_3166_2.toLowerCase());
        }
      }
      if (members.size === 0) throw new Error(`geometry-successors.json: ${row.id}: no parts`);
      composed.set(row.id, { name: row.name, members });
    }
    const topo = build("subdivisions", features, DETAIL.subdivisions, (t, geoms) => {
      const byId = new Map(geoms.map((g) => [g.id, g]));
      const added = [];
      for (const [id, r] of composed) {
        const parts = [...r.members].map((m) => byId.get(m)).filter(Boolean);
        added.push({ ...mergeArcs(t, parts), id, properties: { name: r.name } });
      }
      geoms.push(...added);
      geoms.sort((a, b) => String(a.id).localeCompare(String(b.id)));
      return {
        outline: {
          type: "GeometryCollection",
          geometries: [{ ...mergeArcs(t, [...byId.values()]), id: cc }],
        },
      };
    });
    out.set(cc, topo);
  }
  return out;
}

async function main() {
  const [admin0, admin1] = await Promise.all([
    source(SOURCES.countries),
    source(SOURCES.subdivisions),
  ]);
  const world = countries(admin0);
  const perCountry = subdivisions(admin1, await successors());

  await mkdir(join(OUT, "admin1"), { recursive: true });
  for (const f of await readdir(join(OUT, "admin1"))) {
    if (f.endsWith(".json")) await rm(join(OUT, "admin1", f));
  }
  await writeFile(join(OUT, "world.json"), JSON.stringify(world));
  let total = Buffer.byteLength(JSON.stringify(world));
  for (const cc of [...perCountry.keys()].sort((a, b) => (a < b ? -1 : 1))) {
    const json = JSON.stringify(perCountry.get(cc));
    total += Buffer.byteLength(json);
    await writeFile(join(OUT, "admin1", `${cc}.json`), json);
  }
  process.stdout.write(
    `Natural Earth ${NATURAL_EARTH_VERSION}: ${world.objects.countries.geometries.length} countries, ` +
      `${perCountry.size} countries with subdivisions, ${(total / 1024).toFixed(0)} KiB in all\n`,
  );
}

await main();
