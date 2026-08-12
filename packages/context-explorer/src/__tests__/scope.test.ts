import { describe, expect, it } from "vitest";
import {
  clearDimension,
  contextAddress,
  isRoot,
  ladderOf,
  profileAddress,
  withDimension,
} from "../scope";
import type { Dimension, ScopeTuple } from "../scope";

const PINNED: Dimension[] = ["collection", "path", "coordinate", "locale"];
const FREE: Dimension[] = [
  "workspace",
  "project",
  "stream",
  "collection",
  "path",
  "coordinate",
  "locale",
];

const point: ScopeTuple = {
  workspace: "northsea",
  project: "site",
  stream: "main",
  collection: "docs",
  path: "help/billing.md",
};

describe("contextAddress", () => {
  it("omits the dimensions the mount pins", () => {
    expect(contextAddress(point, PINNED)).toBe("context://docs/help/billing.md");
  });

  it("names the dimensions the mount leaves free", () => {
    expect(contextAddress(point, FREE)).toBe("context://northsea/site@main/docs/help/billing.md");
  });

  it("drops the stream marker when the stream itself is pinned", () => {
    const free: Dimension[] = ["workspace", "project", "collection", "path"];
    expect(contextAddress(point, free)).toBe("context://northsea/site/docs/help/billing.md");
  });

  it("addresses a named profile", () => {
    expect(profileAddress("support")).toBe("context://profile/support");
  });
});

describe("ladderOf", () => {
  it("gives one rung per set dimension and one per path segment", () => {
    const rungs = ladderOf(point, FREE);
    expect(rungs.map((r) => r.value)).toEqual([
      "northsea",
      "site",
      "main",
      "docs",
      "help",
      "billing.md",
    ]);
  });

  it("marks a pinned rung as not navigable", () => {
    const rungs = ladderOf(point, PINNED);
    const byDimension = Object.fromEntries(rungs.map((r) => [r.value, r.navigable]));
    expect(byDimension.northsea).toBe(false);
    expect(byDimension.docs).toBe(true);
  });

  it("addresses each rung at its own point, not the leaf's", () => {
    const rungs = ladderOf(point, FREE);
    expect(rungs[3].address).toBe("context://northsea/site@main/docs");
    expect(rungs[4].address).toBe("context://northsea/site@main/docs/help");
  });

  it("is empty for an empty tuple", () => {
    expect(ladderOf({}, FREE)).toEqual([]);
  });
});

describe("withDimension", () => {
  it("clears every finer ladder rung", () => {
    const next = withDimension(point, "project", "app");
    expect(next).toMatchObject({ workspace: "northsea", project: "app" });
    expect(next.stream).toBeUndefined();
    expect(next.collection).toBeUndefined();
    expect(next.path).toBeUndefined();
  });

  it("leaves the filters alone — they are not rungs", () => {
    const filtered = { ...point, locale: "nb", coordinate: "support/docs" };
    const next = withDimension(filtered, "collection", "strings");
    expect(next.locale).toBe("nb");
    expect(next.coordinate).toBe("support/docs");
  });

  it("treats an empty string as cleared", () => {
    expect(withDimension(point, "collection", "").collection).toBeUndefined();
  });

  it("clearDimension is withDimension of nothing", () => {
    expect(clearDimension(point, "collection")).toEqual(
      withDimension(point, "collection", undefined),
    );
  });
});

describe("isRoot", () => {
  it("is true when nothing the reader may move is set", () => {
    expect(isRoot({ workspace: "northsea" }, ["project", "stream"])).toBe(true);
  });

  it("is false once a free dimension carries a value", () => {
    expect(isRoot({ workspace: "northsea", project: "site" }, ["project"])).toBe(false);
  });
});
