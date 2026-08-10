import { describe, it, expect } from "vite-plus/test";
import {
  coordinateAxes,
  defaultGroupingAxis,
  groupCollectionsByAxis,
  partitionRollups,
  remainingCoordinates,
} from "../components/collections/collectionGrouping";
import type { CollectionTranslationStats } from "../types/api";

function rollup(
  id: string,
  coordinates?: Record<string, string>,
  overrides: Partial<CollectionTranslationStats> = {},
): CollectionTranslationStats {
  return {
    collection_id: id,
    collection_name: id,
    coordinates,
    item_count: 1,
    block_count: 1,
    word_count: 10,
    locales: [],
    ...overrides,
  };
}

describe("partitionRollups", () => {
  it("separates the collection-less bucket from the collections", () => {
    const bucket = rollup("", undefined, { collection_id: "", ungrouped: true });
    const { collections, ungrouped } = partitionRollups([rollup("a"), bucket]);
    expect(collections.map((c) => c.collection_id)).toEqual(["a"]);
    expect(ungrouped).toBe(bucket);
  });

  it("reports no bucket when every item belongs to a collection", () => {
    expect(partitionRollups([rollup("a")]).ungrouped).toBeUndefined();
  });
});

describe("coordinateAxes", () => {
  it("collects every axis any collection carries, alphabetically", () => {
    expect(
      coordinateAxes([
        rollup("a", { product: "bowrain", channel: "app" }),
        rollup("b", { product: "neokapi", market: "de" }),
      ]),
    ).toEqual(["channel", "market", "product"]);
  });

  it("is empty when no collection carries coordinates", () => {
    expect(coordinateAxes([rollup("a"), rollup("b")])).toEqual([]);
  });
});

describe("defaultGroupingAxis", () => {
  it("prefers the axis that cuts equal coverage into fewer groups", () => {
    // Both axes cover all four; product has two values, channel four.
    const collections = [
      rollup("a", { product: "bowrain", channel: "app" }),
      rollup("b", { product: "bowrain", channel: "docs" }),
      rollup("c", { product: "neokapi", channel: "cli" }),
      rollup("d", { product: "neokapi", channel: "engine" }),
    ];
    expect(defaultGroupingAxis(collections)).toBe("product");
  });

  it("prefers the axis that places more collections over the coarser one", () => {
    const collections = [
      rollup("a", { product: "bowrain", tier: "free" }),
      rollup("b", { product: "neokapi" }),
      rollup("c", { product: "neokapi" }),
    ];
    expect(defaultGroupingAxis(collections)).toBe("product");
  });

  it("names no axis when no collection carries coordinates", () => {
    expect(defaultGroupingAxis([rollup("a"), rollup("b")])).toBeUndefined();
  });

  it("ignores an axis every collection leaves empty", () => {
    expect(defaultGroupingAxis([rollup("a", { product: "", channel: "app" })])).toBe("channel");
  });
});

describe("groupCollectionsByAxis", () => {
  it("orders named values alphabetically and the axis-less remainder last", () => {
    const groups = groupCollectionsByAxis(
      [
        rollup("a", { product: "neokapi" }),
        rollup("b"),
        rollup("c", { product: "bowrain" }),
        rollup("d", { product: "neokapi" }),
      ],
      "product",
    );
    expect(groups.map((g) => g.value)).toEqual(["bowrain", "neokapi", ""]);
    expect(groups[1].collections.map((c) => c.collection_id)).toEqual(["a", "d"]);
  });

  it("rolls each group's items and words up", () => {
    const groups = groupCollectionsByAxis(
      [
        rollup("a", { product: "bowrain" }, { item_count: 3, word_count: 100 }),
        rollup("b", { product: "bowrain" }, { item_count: 4, word_count: 250 }),
      ],
      "product",
    );
    expect(groups[0].itemCount).toBe(7);
    expect(groups[0].wordCount).toBe(350);
  });

  it("returns one nameless group when no axis is given", () => {
    const groups = groupCollectionsByAxis([rollup("a", { product: "bowrain" }), rollup("b")]);
    expect(groups).toHaveLength(1);
    expect(groups[0].value).toBe("");
    expect(groups[0].collections).toHaveLength(2);
  });
});

describe("remainingCoordinates", () => {
  it("drops the axis the collection is already filed under", () => {
    expect(
      remainingCoordinates(rollup("a", { product: "bowrain", channel: "app" }), "product"),
    ).toEqual({ channel: "app" });
  });

  it("drops what the channel badge already spells out", () => {
    const c = rollup("a", { product: "bowrain", channel: "app" }, { channel: "bowrain/app" });
    expect(remainingCoordinates(c, "product")).toBeUndefined();
  });

  it("keeps coordinates the channel cannot carry", () => {
    const c = rollup(
      "a",
      { product: "bowrain", channel: "app", market: "de" },
      { channel: "bowrain/app" },
    );
    expect(remainingCoordinates(c, "product")).toEqual({ market: "de" });
  });

  it("is undefined once nothing is left to say", () => {
    expect(remainingCoordinates(rollup("a", { product: "bowrain" }), "product")).toBeUndefined();
    expect(remainingCoordinates(rollup("a"), "product")).toBeUndefined();
  });
});
