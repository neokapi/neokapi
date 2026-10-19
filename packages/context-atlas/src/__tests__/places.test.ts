import { describe, expect, it } from "vite-plus/test";
import containment from "../../../../core/place/testdata/containment.json";
import {
  WORLD,
  bearing,
  commonPlace,
  overlapping,
  placeCounts,
  placeIndex,
  prepareAtlas,
  geoAxis,
} from "../model";
import { makeAtlasFixture, makeOneRecordAtlas } from "../stories/fixtures";
import type { AtlasPlace } from "../types";

// The shared fixture: core/place runs the same cases, so the engine and the
// map read a place the same way.
const shared = placeIndex(containment.places as AtlasPlace[]);

describe("place graph: the shared containment cases", () => {
  it.each(containment.contains)("contains($outer, $inner) is $want", ({ outer, inner, want }) => {
    expect(shared.contains(outer, inner)).toBe(want);
  });

  it.each(containment.overlaps)("overlaps($a, $b) is $want", ({ a, b, want }) => {
    expect(shared.overlaps(a, b)).toBe(want);
    expect(shared.overlaps(b, a)).toBe(want);
  });

  it.each(containment.paths)("path($id)", ({ id, want }) => {
    expect(shared.path(id)).toEqual(want);
  });
});

describe("place graph", () => {
  it("answers nothing for an unknown id but equality", () => {
    expect(shared.path("atlantis")).toEqual([]);
    expect(shared.parent("atlantis")).toBeUndefined();
    expect(shared.children("atlantis")).toEqual([]);
    expect(shared.contains("", "")).toBe(false);
    expect(shared.overlaps("", "oslo")).toBe(false);
  });

  it("lists children by path parent, with the members of a place made of others, by name", () => {
    expect(shared.children("no-03")).toEqual(["oslo", "uni-district"]);
    expect(shared.children("oslo")).toEqual(["campus", "grunerlokka"]);
    expect(shared.children("nordics")).toEqual(["dk", "fi", "is", "no", "se"]);
    expect(shared.children("northern-europe")).toEqual([
      "dk",
      "fi",
      "is",
      "nordics",
      "no",
      "scandinavia",
      "se",
    ]);
    expect(shared.children("no")).toEqual(["no-32", "greater-oslo", "no-03", "no-46"]);
    expect(shared.parent("greater-oslo")).toBe("no");
    expect(shared.parent(WORLD)).toBeUndefined();
  });

  it("sorts places by id, keeps the first of a repeated id and tolerates a missing list", () => {
    const g = placeIndex([
      { id: "b", name: "B", type: "area", within: ["world"] },
      { id: "a", name: "A", type: "area", within: ["world"] },
      { id: "a", name: "Second", type: "area", within: ["world"] },
      { id: "world", name: "World", type: "world" },
    ]);
    expect(g.places.map((p) => p.id)).toEqual(["a", "b", "world"]);
    expect(g.get("a")?.name).toBe("A");
    expect(placeIndex(undefined).places).toEqual([]);
  });

  it("answers false on a cycle instead of recursing", () => {
    const g = placeIndex([
      { id: "world", name: "World", type: "world" },
      { id: "x", name: "X", type: "area", within: ["y"] },
      { id: "y", name: "Y", type: "area", within: ["x"] },
      { id: "m", name: "M", type: "territory", of: ["m"] },
    ]);
    expect(g.contains("y", "x")).toBe(true);
    expect(g.contains("z", "x")).toBe(false);
    expect(g.path("x")).toEqual([]);
    expect(g.contains("x", "m")).toBe(false);
    expect(g.overlaps("m", "x")).toBe(false);
  });

  it("says how a record bears on a drawn place", () => {
    expect(bearing(shared, "no-03", "oslo")).toBe("sits");
    expect(bearing(shared, "no-03", "no-03")).toBe("sits");
    expect(bearing(shared, "no-03", "nordics")).toBe("held");
    expect(bearing(shared, "no-03", "greater-oslo")).toBe("held");
    expect(bearing(shared, "oslo", "uni-district")).toBe("held");
    expect(bearing(shared, "grunerlokka", "uni-district")).toBeUndefined();
    expect(bearing(shared, "no-46", "greater-oslo")).toBeUndefined();
    expect(bearing(shared, "no", WORLD)).toBeUndefined();
    expect(bearing(shared, "no", "")).toBeUndefined();
  });

  it("folds per-place tallies onto drawn places", () => {
    const byPlace = new Map([
      ["oslo", { weight: 3, attention: 1, around: 4 }],
      ["nordics", { weight: 2, attention: 0, around: 2 }],
      ["world", { weight: 5, attention: 5, around: 5 }],
      ["bergen", { weight: 1, attention: 0, around: 1 }],
    ]);
    const c = placeCounts(shared, ["no-03", "no-46", "de"], byPlace);
    expect(c.get("no-03")).toEqual({
      sits: { weight: 3, attention: 1, around: 4 },
      held: { weight: 2, attention: 0, around: 2 },
      heldThrough: ["nordics"],
    });
    expect(c.get("no-46")?.sits.weight).toBe(1);
    expect(c.get("de")).toEqual({
      sits: { weight: 0, attention: 0, around: 0 },
      held: { weight: 0, attention: 0, around: 0 },
      heldThrough: [],
    });
  });

  it("selects every value that shares ground with a place", () => {
    const values = ["oslo", "bergen", "nordics", "greater-oslo", "world", "de", ""];
    expect(overlapping(shared, "no-03", values)).toEqual([
      "oslo",
      "nordics",
      "greater-oslo",
      "world",
    ]);
    expect(overlapping(shared, "de", values)).toEqual(["world", "de"]);
  });

  it("opens the map at the narrowest region, country or territory that holds every place", () => {
    expect(commonPlace(shared, ["oslo", "bergen"])).toBe("no");
    expect(commonPlace(shared, ["oslo", "nordics"])).toBe("nordics");
    expect(commonPlace(shared, ["oslo", "se"])).toBe("scandinavia");
    expect(commonPlace(shared, ["oslo", "fi"])).toBe("nordics");
    // Greater Oslo is made of counties, so the map opens no deeper than Norway.
    expect(commonPlace(shared, ["oslo", "no-32"])).toBe("no");
    expect(commonPlace(shared, ["oslo", "de"])).toBe("europe");
    expect(commonPlace(shared, ["world", "atlantis"])).toBe(WORLD);
    expect(commonPlace(shared, [])).toBe(WORLD);
  });
});

describe("place graph in the atlas", () => {
  const atlas = prepareAtlas(makeAtlasFixture());

  it("carries the places the records name, with their ancestors, members and children", () => {
    const g = atlas.places;
    for (const id of ["world", "europe", "northern-europe", "no", "no-03", "no-32", "oslo"]) {
      expect(g.has(id), id).toBe(true);
    }
    expect(g.path("grunerlokka")).toEqual([
      "world",
      "europe",
      "northern-europe",
      "no",
      "no-03",
      "oslo",
      "grunerlokka",
    ]);
    expect(g.contains("nordics", "bergen")).toBe(true);
    expect(g.contains("europe", "pt")).toBe(true);
    const ids = g.places.map((p) => p.id);
    expect([...ids].sort()).toEqual(ids);
  });

  it("offers the map on a geo axis whose values the graph knows", () => {
    expect(geoAxis(atlas)?.id).toBe("market");
    expect(geoAxis(prepareAtlas(makeOneRecordAtlas()))).toBeUndefined();
  });
});
