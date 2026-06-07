import { describe, expect, it } from "vitest";
import { treeToRenderDoc, parseCellRef, colLabel, runsText } from "@neokapi/ui-primitives/preview";
import type { ContentNode, ContentTree, Run } from "@neokapi/ui-primitives/preview";

// Fixture trees mirror the REAL `kapi inspect` output (verified against the
// engine) for the three Office/text shapes the renderers target. The key facts
// under test: pptx renders only ppt/slides/* (never slideLayouts/slideMasters),
// xlsx places cells from properties.cell into a grid (incl. blank gaps), docx
// renders word/document.xml paragraphs, and unknown formats fall back to a list.

function txt(s: string): Run[] {
  return [{ text: s }];
}

function block(
  id: string,
  type: string,
  source: string,
  props?: Record<string, string>,
): ContentNode {
  return { kind: "block", id, type, source: txt(source), properties: props };
}

function layer(name: string, children: ContentNode[]): ContentNode {
  return { kind: "layer", id: name, name, children };
}

function tree(format: string, root: ContentNode[]): ContentTree {
  return { format, root, stats: { layers: 0, groups: 0, blocks: 0, data: 0, media: 0, runs: 0 } };
}

describe("runsText", () => {
  it("concatenates literal text runs and ignores markup runs", () => {
    const runs: Run[] = [{ text: "Hello " }, { ph: { id: "1" } }, { text: "world" }];
    expect(runsText(runs)).toBe("Hello world");
  });
});

describe("parseCellRef / colLabel", () => {
  it("parses single and multi-letter refs to zero-based positions", () => {
    expect(parseCellRef("A1")).toEqual({ col: 0, row: 0 });
    expect(parseCellRef("B3")).toEqual({ col: 1, row: 2 });
    expect(parseCellRef("AA10")).toEqual({ col: 26, row: 9 });
    expect(parseCellRef("nope")).toBeNull();
  });
  it("round-trips column labels", () => {
    expect(colLabel(0)).toBe("A");
    expect(colLabel(1)).toBe("B");
    expect(colLabel(26)).toBe("AA");
  });
});

describe("treeToRenderDoc — PPTX", () => {
  // The real openxml pptx tree: a slide layer with the content, plus
  // slideLayouts/slideMasters/docProps boilerplate that MUST be ignored.
  const pptx = tree("openxml", [
    layer("/tmp/deck.pptx", [
      layer("ppt/slides/slide1.xml", [
        block("tu1", "paragraph", "Welcome to Acme", { partPath: "ppt/slides/slide1.xml" }),
        block("tu2", "paragraph", "Acme makes every quarter count.", {}),
        block("tu3", "paragraph", "Sign up for Acme today", {}),
      ]),
      layer("ppt/slideLayouts/slideLayout1.xml", [
        block("tu9", "paragraph", "Click to edit Master title style", {}),
      ]),
      layer("ppt/slideMasters/slideMaster1.xml", [
        block("tu20", "paragraph", "Click to edit Master text styles", {}),
      ]),
      layer("docProps/core.xml", [block("tu30", "property", "generated using python-pptx", {})]),
    ]),
  ]);

  it("renders only ppt/slides/* as a slide deck, first block = title", () => {
    const doc = treeToRenderDoc(pptx);
    expect(doc.kind).toBe("slides");
    expect(doc.slides).toHaveLength(1);
    const slide = doc.slides![0];
    expect(slide.name).toBe("ppt/slides/slide1.xml");
    expect(slide.title?.text).toBe("Welcome to Acme");
    expect(slide.title?.id).toBe("tu1");
    expect(slide.bullets.map((b) => b.text)).toEqual([
      "Acme makes every quarter count.",
      "Sign up for Acme today",
    ]);
  });

  it("never includes slideLayouts / slideMasters / docProps boilerplate", () => {
    const doc = treeToRenderDoc(pptx);
    const allText = (doc.slides ?? [])
      .flatMap((s) => [s.title?.text ?? "", ...s.bullets.map((b) => b.text)])
      .join(" ");
    expect(allText).not.toContain("Master");
    expect(allText).not.toContain("python-pptx");
  });

  it("orders slides by trailing number (slide2 before slide10)", () => {
    const multi = tree("openxml", [
      layer("/tmp/d.pptx", [
        layer("ppt/slides/slide10.xml", [block("a", "paragraph", "Ten", {})]),
        layer("ppt/slides/slide2.xml", [block("b", "paragraph", "Two", {})]),
      ]),
    ]);
    const doc = treeToRenderDoc(multi);
    expect(doc.slides!.map((s) => s.title?.text)).toEqual(["Two", "Ten"]);
  });
});

describe("treeToRenderDoc — XLSX", () => {
  // sheet1 has A1/B1/A2/B2 populated but B3 BLANK (only A3 present) so the grid
  // must leave a gap. docProps is boilerplate and ignored.
  const xlsx = tree("openxml", [
    layer("/tmp/report.xlsx", [
      layer("xl/worksheets/sheet1.xml", [
        block("tu1", "cell", "Acme quarterly revenue", { cell: "A1" }),
        block("tu2", "cell", "Total revenue", { cell: "B1" }),
        block("tu3", "cell", "Acme net profit", { cell: "A2" }),
        block("tu4", "cell", "Net profit", { cell: "B2" }),
        block("tu5", "cell", "Acme customer count", { cell: "A3" }),
      ]),
      layer("docProps/core.xml", [block("tu7", "property", "openpyxl", {})]),
    ]),
  ]);

  it("renders a sheet grid placed by cell ref, ignoring docProps", () => {
    const doc = treeToRenderDoc(xlsx);
    expect(doc.kind).toBe("sheet");
    expect(doc.sheet!.name).toBe("xl/worksheets/sheet1.xml");
    expect(doc.sheet!.cols).toBe(2);
    expect(doc.sheet!.rows).toBe(3);
    const a1 = doc.sheet!.cells.find((c) => c.ref === "A1");
    expect(a1).toMatchObject({ col: 0, row: 0, text: "Acme quarterly revenue", id: "tu1" });
    // No cell exists for the blank B3.
    expect(doc.sheet!.cells.find((c) => c.col === 1 && c.row === 2)).toBeUndefined();
    expect(doc.sheet!.cells.find((c) => c.text === "openpyxl")).toBeUndefined();
  });
});

describe("treeToRenderDoc — DOCX", () => {
  const docx = tree("openxml", [
    layer("/tmp/welcome.docx", [
      layer("word/document.xml", [
        block("tu1", "paragraph", "Welcome to Acme", {}),
        block("tu2", "paragraph", "Your account is ready.", {}),
      ]),
    ]),
  ]);

  it("renders word/document.xml paragraphs, first as a heading", () => {
    const doc = treeToRenderDoc(docx);
    expect(doc.kind).toBe("doc");
    expect(doc.paragraphs!.map((p) => p.text)).toEqual([
      "Welcome to Acme",
      "Your account is ready.",
    ]);
    expect(doc.paragraphs![0].role).toBe("heading");
    expect(doc.paragraphs![1].role).toBe("body");
  });
});

describe("treeToRenderDoc — Markdown & fallback", () => {
  it("renders markdown blocks as a doc with heading/bullet roles", () => {
    const md = tree("markdown", [
      layer("/tmp/guide.md", [
        block("tu1", "heading", "Welcome to Acme", {}),
        block("tu2", "", "Acme helps teams ship faster.", {}),
        block("tu3", "list-item", "Sign up for Acme today", {}),
      ]),
    ]);
    const doc = treeToRenderDoc(md);
    expect(doc.kind).toBe("doc");
    expect(doc.paragraphs!.map((p) => p.role)).toEqual(["heading", "body", "bullet"]);
  });

  it("falls back to a key list for JSON / unknown catalog formats", () => {
    const json = tree("json", [
      layer("/tmp/messages.json", [
        block("tu1", "", "Welcome to Acme", {}),
        block("tu2", "", "Sign up today", {}),
      ]),
    ]);
    const doc = treeToRenderDoc(json);
    expect(doc.kind).toBe("list");
    expect(doc.lines!.map((l) => l.text)).toEqual(["Welcome to Acme", "Sign up today"]);
  });
});
