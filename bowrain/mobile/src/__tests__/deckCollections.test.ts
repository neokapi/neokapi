import type { DeckCollection } from "@neokapi/bowrain-client";

import {
  collectionFromParam,
  collectionParam,
  decidedShare,
  nextCollection,
  startCollection,
} from "../lib/deckCollections";

function coll(id: string, waiting: number): DeckCollection {
  return {
    collection_id: id,
    name: id || "",
    waiting,
    fast: 0,
    closer: waiting,
    desk: 0,
    unclassified: 0,
    translations: 10,
    decided: 10 - waiting,
  };
}

describe("deck collections", () => {
  it.each([
    ["c1", "c1"],
    ["", "-"],
  ])("carries collection %j in the route as %j and back", (id, param) => {
    expect(collectionParam(id)).toBe(param);
    expect(collectionFromParam(param)).toBe(id);
  });

  it.each([[undefined], [""], [[]]])("reads %j as the whole language", (param) => {
    expect(collectionFromParam(param as string | string[] | undefined)).toBeUndefined();
  });

  it("offers the first other collection with anything waiting, in the server's order", () => {
    const list = [coll("web", 4), coll("docs", 2), coll("", 1), coll("emails", 0)];
    expect(nextCollection(list, "web")?.collection_id).toBe("docs");
    expect(nextCollection(list, "docs")?.collection_id).toBe("web");
    expect(nextCollection([coll("web", 3), coll("emails", 0)], "web")).toBeNull();
    expect(nextCollection(list, undefined)).toBeNull();
    expect(nextCollection(undefined, "web")).toBeNull();
  });

  it("measures progress as decided of all translations", () => {
    expect(decidedShare({ decided: 3, translations: 4 })).toBe(0.75);
    expect(decidedShare({ decided: 0, translations: 0 })).toBe(0);
  });
});

describe("startCollection", () => {
  const base = {
    name: "",
    fast: 0,
    closer: 0,
    desk: 0,
    unclassified: 0,
    translations: 1,
    decided: 0,
  };
  it("starts with the first waiting high priority collection", () => {
    expect(
      startCollection([
        { ...base, collection_id: "a", waiting: 0, priority: "high" },
        { ...base, collection_id: "b", waiting: 3, priority: "high" },
        { ...base, collection_id: "c", waiting: 5, priority: "normal" },
      ]),
    ).toBe("b");
  });
  it("marks nothing when no waiting collection is high priority", () => {
    expect(
      startCollection([{ ...base, collection_id: "c", waiting: 5, priority: "normal" }]),
    ).toBeNull();
    expect(startCollection(undefined)).toBeNull();
  });
});
