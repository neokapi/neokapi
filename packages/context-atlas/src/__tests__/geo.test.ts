import { readFileSync, readdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vite-plus/test";
import { geoArea } from "d3-geo";
import { feature } from "topojson-client";
import type { GeometryCollection, Topology } from "topojson-specification";
import { loadCountries, loadSubdivisions } from "../geo";

const GEO = join(dirname(fileURLToPath(import.meta.url)), "..", "geo");

function read(path: string): Topology {
  return JSON.parse(readFileSync(path, "utf8")) as Topology;
}

function features(topo: Topology, name: string) {
  return feature(topo, topo.objects[name] as GeometryCollection).features;
}

describe("built-in geometry", () => {
  it("keys the world's countries by lower-case ISO 3166-1 alpha-2, each smaller than a hemisphere", () => {
    const world = features(read(join(GEO, "world.json")), "countries");
    const keyed = world.filter((f) => f.id !== undefined);
    expect(keyed.length).toBeGreaterThan(200);
    for (const f of keyed) expect(String(f.id)).toMatch(/^[a-z]{2}$/u);
    expect(new Set(keyed.map((f) => f.id)).size).toBe(keyed.length);
    for (const f of world) expect(geoArea(f)).toBeLessThan(2 * Math.PI);
    expect(keyed.map((f) => f.id)).toEqual(expect.arrayContaining(["no", "fr", "se", "us"]));
  });

  it("keys each country's subdivisions by lower-case ISO 3166-2 and winds every ring for d3", () => {
    const files = readdirSync(join(GEO, "admin1")).filter((f) => f.endsWith(".json"));
    expect(files.length).toBeGreaterThan(150);
    for (const file of files) {
      const cc = file.replace(/\.json$/u, "");
      const topo = read(join(GEO, "admin1", file));
      const subs = features(topo, "subdivisions");
      for (const f of subs) {
        expect(String(f.id).startsWith(`${cc}-`), `${file}: ${String(f.id)}`).toBe(true);
        expect(geoArea(f), `${file}: ${String(f.id)}`).toBeLessThan(2 * Math.PI);
      }
      const outline = features(topo, "outline");
      expect(outline.map((f) => f.id)).toEqual([cc]);
      expect(geoArea(outline[0])).toBeLessThan(2 * Math.PI);
    }
  });

  it("stays small enough to ship", () => {
    const world = readFileSync(join(GEO, "world.json")).length;
    const all = readdirSync(join(GEO, "admin1")).reduce(
      (n, f) => n + readFileSync(join(GEO, "admin1", f)).length,
      world,
    );
    expect(world).toBeLessThan(300 * 1024);
    expect(all).toBeLessThan(5 * 1024 * 1024);
  });

  it("derives a region's shape from the subdivisions Natural Earth files under it", async () => {
    const fr = await loadSubdivisions("fr");
    expect(fr?.has("fr-ara")).toBe(true);
    expect(fr?.has("fr-69")).toBe(true);
  });

  it("composes current ids from the features Natural Earth carries under older codes", async () => {
    const no = await loadSubdivisions("no");
    const current = [
      "03",
      "11",
      "15",
      "18",
      "31",
      "32",
      "33",
      "34",
      "39",
      "40",
      "42",
      "46",
      "50",
      "55",
      "56",
    ];
    for (const n of current) expect(no?.has(`no-${n}`), `no-${n}`).toBe(true);
    // Innlandet is Hedmark and Oppland together: larger than either.
    const area = (id: string) => geoArea(no!.get(id)!);
    expect(area("no-34")).toBeGreaterThan(area("no-04"));
    expect(area("no-34")).toBeCloseTo(area("no-04") + area("no-05"), 6);
    const pl = await loadSubdivisions("pl");
    expect(pl?.has("pl-14")).toBe(true);
    const gb = await loadSubdivisions("gb");
    expect(gb?.get("gb-sct")?.properties?.name).toBe("Scotland");
  });

  it("names a source for every successor, and composes it from codes Natural Earth carries", () => {
    const table = JSON.parse(
      readFileSync(join(GEO, "..", "..", "scripts", "geometry-successors.json"), "utf8"),
    ) as { successors: { id: string; from?: string[]; where?: object; source: string }[] };
    const ids = new Set<string>();
    for (const row of table.successors) {
      expect(row.id).toMatch(/^[a-z]{2}-[a-z0-9]{1,3}$/u);
      expect(ids.has(row.id), row.id).toBe(false);
      ids.add(row.id);
      expect(row.source.length, row.id).toBeGreaterThan(20);
      expect(!!row.from !== !!row.where, row.id).toBe(true);
      const file = read(join(GEO, "admin1", `${row.id.slice(0, 2)}.json`));
      expect(
        features(file, "subdivisions").some((f) => f.id === row.id),
        row.id,
      ).toBe(true);
    }
  });

  it("loads lazily by country and answers nothing for a country without a file", async () => {
    const world = await loadCountries();
    expect(world.get("no")?.properties?.name).toBe("Norway");
    const no = await loadSubdivisions("no");
    expect(no?.get("no-03")?.properties?.name).toBe("Oslo");
    expect(no?.outline).toBeDefined();
    expect(await loadSubdivisions("zz")).toBeNull();
  });
});
