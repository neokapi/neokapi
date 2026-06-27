import { describe, expect, it } from "vitest";
import { headingLevel, inlineSegments, nodeText, tableFromNode } from "./projectionRender";
import type { RenderNode, Run } from "./types";

describe("inlineSegments", () => {
  it("decodes bold + link with href, mirroring Go WalkInline", () => {
    const runs: Run[] = [
      { text: "Some " },
      { pcOpen: { id: "1", type: "fmt:bold" } },
      { text: "bold" },
      { pcClose: { id: "1", type: "fmt:bold" } },
      { text: " and a " },
      { pcOpen: { id: "2", type: "link:hyperlink", attrs: { href: "https://x.com" } } },
      { text: "link" },
      { pcClose: { id: "2", type: "link:hyperlink" } },
    ];
    expect(inlineSegments(runs)).toEqual([
      { kind: "text", text: "Some " },
      { kind: "open", type: "fmt:bold", attrs: undefined },
      { kind: "text", text: "bold" },
      { kind: "close", type: "fmt:bold" },
      { kind: "text", text: " and a " },
      { kind: "open", type: "link:hyperlink", attrs: { href: "https://x.com" } },
      { kind: "text", text: "link" },
      { kind: "close", type: "link:hyperlink" },
    ]);
  });

  it("decodes an image placeholder with src/alt", () => {
    const runs: Run[] = [
      {
        ph: { id: "i", type: "media:image", equiv: "logo", attrs: { src: "/l.png", alt: "Logo" } },
      },
    ];
    expect(inlineSegments(runs)).toEqual([
      {
        kind: "placeholder",
        type: "media:image",
        equiv: "logo",
        attrs: { src: "/l.png", alt: "Logo" },
      },
    ]);
  });

  it("takes the 'other' plural branch", () => {
    const runs: Run[] = [
      { plural: { pivot: "n", forms: { one: [{ text: "one" }], other: [{ text: "many" }] } } },
    ];
    expect(inlineSegments(runs)).toEqual([{ kind: "text", text: "many" }]);
  });

  it("is empty for undefined runs", () => {
    expect(inlineSegments(undefined)).toEqual([]);
  });
});

describe("tableFromNode", () => {
  const cell = (text: string, header = false, extra: Partial<RenderNode> = {}): RenderNode => ({
    role: header ? "table-header" : "table-cell",
    runs: [{ text }],
    header,
    ...extra,
  });
  const table: RenderNode = {
    role: "table",
    children: [
      { role: "table-row", children: [cell("A", true), cell("B", true)] },
      { role: "table-row", children: [cell("1"), cell("2", false, { colSpan: 2 })] },
    ],
  };

  it("reads rows-of-cells with header + span info", () => {
    const t = tableFromNode(table);
    expect(t).not.toBeNull();
    expect(t!.rows).toHaveLength(2);
    expect(t!.rows[0][0].header).toBe(true);
    expect(nodeText(t!.rows[0][0].node)).toBe("A");
    expect(t!.rows[1][1].colSpan).toBe(2);
    expect(t!.rows[1][0].header).toBe(false);
  });

  it("returns null for a non-table node", () => {
    expect(tableFromNode({ role: "paragraph", runs: [{ text: "x" }] })).toBeNull();
  });
});

describe("headingLevel", () => {
  it("returns the level for headings and defaults title to 1", () => {
    expect(headingLevel({ role: "heading", level: 3 })).toBe(3);
    expect(headingLevel({ role: "title" })).toBe(1);
    expect(headingLevel({ role: "heading" })).toBe(2);
    expect(headingLevel({ role: "paragraph" })).toBeNull();
  });
});
