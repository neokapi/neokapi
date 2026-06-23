import { describe, it, expect } from "vitest";
import type { ContentNode, ContentTree } from "./types";
import { treeToRenderDoc, entryKey } from "./renderDoc";

function cell(ref: string, text: string, siIndex?: string): ContentNode {
  return {
    kind: "block",
    id: `cell-sheet1-${ref}`,
    source: [{ text }],
    properties: {
      partPath: "xl/worksheets/sheet1.xml",
      cell: ref,
      ...(siIndex !== undefined ? { siIndex } : {}),
    },
  } as ContentNode;
}

function sharedString(
  siIndex: string,
  text: string,
  targets?: Record<string, string>,
): ContentNode {
  return {
    kind: "block",
    id: `tu-si-${siIndex}`,
    source: [{ text }],
    properties: { partPath: "xl/sharedStrings.xml", siIndex },
    ...(targets
      ? { targets: Object.fromEntries(Object.entries(targets).map(([l, t]) => [l, [{ text: t }]])) }
      : {}),
  } as ContentNode;
}

function sheetTree(cells: ContentNode[], shared: ContentNode[] = []): ContentTree {
  const root: ContentNode[] = [
    { kind: "layer", id: "L", name: "xl/worksheets/sheet1.xml", children: cells } as ContentNode,
  ];
  if (shared.length > 0) {
    root.push({
      kind: "layer",
      id: "SS",
      name: "xl/sharedStrings.xml",
      children: shared,
    } as ContentNode);
  }
  return { format: "openxml", root } as ContentTree;
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

  it("joins cell anchors to their shared-string block for targets", () => {
    // The translatable text + its translations live once in sharedStrings.xml;
    // the worksheet cell only references it by siIndex. The grid should render
    // the shared-string block's source and targets, keyed to the cell position.
    const doc = treeToRenderDoc(
      sheetTree(
        [cell("A1", "0", "0"), cell("B1", "1", "1")],
        [
          sharedString("0", "Hello", { "fr-FR": "Bonjour" }),
          sharedString("1", "World", { "fr-FR": "Monde" }),
        ],
      ),
    );
    expect(doc.kind).toBe("sheet");
    const a1 = doc.sheet!.cells.find((c) => c.ref === "A1")!;
    const b1 = doc.sheet!.cells.find((c) => c.ref === "B1")!;
    // Source resolves from the shared-string block (not the raw "0"/"1" index).
    expect(a1.text).toBe("Hello");
    expect(b1.text).toBe("World");
    // Targets ride along so the grid can show translations.
    expect(a1.targets?.["fr-FR"]).toBe("Bonjour");
    expect(b1.targets?.["fr-FR"]).toBe("Monde");
    // Each cell keeps its own id (one shared string backs many cells).
    expect(a1.id).toBe("cell-sheet1-A1");
    expect(b1.id).toBe("cell-sheet1-B1");
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
