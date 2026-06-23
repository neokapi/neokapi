import { describe, it, expect } from "vitest";
import type { ContentNode, ContentTree } from "./types";
import { treeToRenderDoc, entryKey } from "./renderDoc";

function cell(ref: string, text: string, translatable = false): ContentNode {
  return {
    kind: "block",
    id: `cell-sheet1-${ref}`,
    source: [{ text }],
    properties: { partPath: "xl/worksheets/sheet1.xml", cell: ref },
    ...(translatable ? {} : {}),
  } as ContentNode;
}

function sheetTree(cells: ContentNode[]): ContentTree {
  return {
    format: "openxml",
    root: [{ kind: "layer", id: "L", name: "xl/worksheets/sheet1.xml", children: cells }],
  } as ContentTree;
}

describe("renderDoc — xlsx grid from cell anchors", () => {
  it("reconstructs a worksheet grid from cell-anchored blocks (shared-string case)", () => {
    // Shared-string cells are non-translatable grid anchors emitted by the SML
    // reader; they carry properties.cell + the resolved text, which is enough to
    // place them in a grid even though the translatable text lives once in
    // sharedStrings.xml.
    const doc = treeToRenderDoc(
      sheetTree([cell("A1", "ID"), cell("B1", "Artist"), cell("B2", "Nordlys")]),
    );
    expect(doc.kind).toBe("sheet");
    expect(doc.sheet).toBeDefined();
    const placed = doc.sheet!.cells.map((c) => `${c.ref}=${c.text}@${c.col},${c.row}`);
    expect(placed).toContain("A1=ID@0,0");
    expect(placed).toContain("B1=Artist@1,0");
    expect(placed).toContain("B2=Nordlys@1,1");
    // The grid spans the populated extent (cols A..B, rows 1..2).
    expect(doc.sheet!.cols).toBe(2);
    expect(doc.sheet!.rows).toBe(2);
  });
});

describe("renderDoc — entryKey surfaces structured keys", () => {
  it("falls back to json.keypath and the block name for catalog formats", () => {
    expect(entryKey({ kind: "block", id: "1", properties: { key: "a.b" } } as ContentNode)).toBe(
      "a.b",
    );
    expect(
      entryKey({
        kind: "block",
        id: "2",
        properties: { "json.keypath": "cart.title" },
      } as ContentNode),
    ).toBe("cart.title");
    expect(entryKey({ kind: "block", id: "3", name: "checkout.button" } as ContentNode)).toBe(
      "checkout.button",
    );
    expect(entryKey({ kind: "block", id: "4" } as ContentNode)).toBeUndefined();
  });
});
