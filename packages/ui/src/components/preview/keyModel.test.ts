import { describe, expect, it } from "vitest";
import { groupRows, groupSize, keyedRows, rowLocales, splitKeyPath } from "./keyModel";
import type { ContentNode, ContentTree } from "./types";

function block(name: string, over: Partial<ContentNode> = {}): ContentNode {
  return {
    kind: "block",
    id: `b:${name}`,
    name,
    source: [{ text: name }],
    ...over,
  };
}

function tree(nodes: ContentNode[], format = "json"): ContentTree {
  return {
    format,
    root: nodes,
    stats: { layers: 0, groups: 0, blocks: nodes.length, data: 0, media: 0, runs: 0 },
  };
}

describe("splitKeyPath", () => {
  it("splits a dotted JSON path", () => {
    expect(splitKeyPath("errors.network.timeout")).toEqual(["errors", "network", "timeout"]);
  });

  it("drops a JSONPath root, which names the document rather than a level", () => {
    expect(splitKeyPath("$.errors.network.timeout")).toEqual(["errors", "network", "timeout"]);
    expect(splitKeyPath("$")).toEqual([]);
  });

  it("splits a JSON pointer", () => {
    expect(splitKeyPath("/errors/network/timeout")).toEqual(["errors", "network", "timeout"]);
  });

  it("keeps a flat properties key as one segment", () => {
    expect(splitKeyPath("app_title")).toEqual(["app_title"]);
  });

  it("breaks an array index into its own segment", () => {
    expect(splitKeyPath("items[0].label")).toEqual(["items", "0", "label"]);
    expect(splitKeyPath("grid[2][3]")).toEqual(["grid", "2", "3"]);
  });

  it("drops empty segments", () => {
    expect(splitKeyPath("")).toEqual([]);
    expect(splitKeyPath("a..b")).toEqual(["a", "b"]);
  });
});

describe("keyedRows", () => {
  it("takes the key from the block name the reader assigned", () => {
    const rows = keyedRows(tree([block("errors.network.timeout")]));
    expect(rows).toHaveLength(1);
    expect(rows[0].key).toBe("errors.network.timeout");
    expect(rows[0].leaf).toBe("timeout");
    expect(rows[0].id).toBe("b:errors.network.timeout");
  });

  it("prefers a key property over the name", () => {
    const rows = keyedRows(
      tree([{ kind: "block", id: "b1", name: "ignored", properties: { key: "app.title" } }]),
    );
    expect(rows[0].key).toBe("app.title");
  });

  it("falls back to the block id so no unit is unlabelled", () => {
    const rows = keyedRows(tree([{ kind: "block", id: "b1" }]));
    expect(rows[0].key).toBe("b1");
    expect(rows[0].leaf).toBe("b1");
  });

  it("walks blocks nested under layers, in document order", () => {
    const rows = keyedRows(
      tree([
        {
          kind: "layer",
          id: "messages.json",
          name: "messages.json",
          children: [block("a"), block("b")],
        },
      ]),
    );
    expect(rows.map((r) => r.key)).toEqual(["a", "b"]);
  });

  it("carries every locale's target runs", () => {
    const rows = keyedRows(
      tree([block("greeting", { targets: { fr: [{ text: "Bonjour" }], nb: [{ text: "Hei" }] } })]),
    );
    expect(rowLocales(rows)).toEqual(["fr", "nb"]);
    expect(rows[0].targets.fr).toEqual([{ text: "Bonjour" }]);
  });
});

describe("groupRows", () => {
  it("nests a dotted JSON path under each of its prefixes", () => {
    const root = groupRows(
      keyedRows(tree([block("errors.network.timeout"), block("errors.network.refused")])),
    );
    expect(root.rows).toHaveLength(0);
    expect(root.groups).toHaveLength(1);
    const errors = root.groups[0];
    expect(errors.path).toEqual(["errors"]);
    expect(errors.groups).toHaveLength(1);
    const network = errors.groups[0];
    expect(network.path).toEqual(["errors", "network"]);
    expect(network.rows.map((r) => r.leaf)).toEqual(["timeout", "refused"]);
  });

  it("renders a flat properties file as one group", () => {
    const root = groupRows(
      keyedRows(tree([block("app_title"), block("app_subtitle")], "properties")),
    );
    expect(root.groups).toHaveLength(0);
    expect(root.rows.map((r) => r.key)).toEqual(["app_title", "app_subtitle"]);
  });

  it("groups array elements under their container", () => {
    const root = groupRows(keyedRows(tree([block("items[0].label"), block("items[1].label")])));
    const items = root.groups[0];
    expect(items.path).toEqual(["items"]);
    expect(items.groups.map((g) => g.path)).toEqual([
      ["items", "0"],
      ["items", "1"],
    ]);
    expect(groupSize(items)).toBe(2);
  });

  it("keeps sibling groups in first-seen order and rows in document order", () => {
    const root = groupRows(keyedRows(tree([block("b.one"), block("a.one"), block("b.two")])));
    expect(root.groups.map((g) => g.path[0])).toEqual(["b", "a"]);
    expect(root.groups[0].rows.map((r) => r.leaf)).toEqual(["one", "two"]);
  });

  it("puts a top-level key beside the groups rather than inside one", () => {
    const root = groupRows(keyedRows(tree([block("title"), block("errors.timeout")])));
    expect(root.rows.map((r) => r.key)).toEqual(["title"]);
    expect(root.groups.map((g) => g.path)).toEqual([["errors"]]);
  });
});
