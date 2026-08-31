// @vitest-environment jsdom
//
// The "Blocks", "Structure" and "Layout" tabs of DocumentViewer are other
// places (besides FormatPreview's "Preview" tab) that render a block's own
// source/target prose to a reader. Each needs the same contract: the element
// whose text-align actually governs the row — not just some inline
// descendant — carries dir/lang for that text's locale.
import { describe, it, expect } from "vitest";
import { createElement } from "react";
import { createRoot } from "react-dom/client";
import { act } from "react";

import BlockInspector from "../components/preview/BlockInspector";
import StructureView from "../components/preview/StructureView";
import LayoutView from "../components/preview/LayoutView";
import type { ContentNode, ContentTree, OverlayView } from "../components/preview/types";

function renderEl(el: React.ReactElement): HTMLDivElement {
  const container = document.createElement("div");
  document.body.appendChild(container);
  act(() => {
    createRoot(container).render(el);
  });
  return container;
}

const arabicBlock: ContentNode = {
  kind: "block",
  id: "b1",
  type: "paragraph",
  translatable: true,
  sourceLocale: "ar",
  source: [{ text: "مرحباً بك في كابي مارت" }],
  overlays: [
    {
      type: "term",
      side: "source",
      spans: [{ range: { start: "r0:0", end: "r0:6" }, text: "مرحباً" }],
    } as OverlayView,
  ],
};

describe("BlockInspector — writing direction", () => {
  it("puts dir/lang on the collapsed row's text preview, not just inside it", () => {
    const c = renderEl(createElement(BlockInspector, { node: arabicBlock }));
    // The collapsed summary is the block's own truncated preview span — find it
    // by its known class rather than assuming DOM position.
    const summary = c.querySelector(".truncate");
    expect(summary?.getAttribute("dir")).toBe("rtl");
    expect(summary?.getAttribute("lang")).toBe("ar");
    expect(summary?.textContent).toContain("مرحباً");
  });

  it("puts dir/lang on a target row keyed by its own variant locale", () => {
    const block: ContentNode = {
      ...arabicBlock,
      sourceLocale: "en",
      source: [{ text: "Welcome to Kapi Mart" }],
      targets: { "ar-EG#formal": [{ text: "مرحباً بك في كابي مارت" }] },
    };
    const c = renderEl(createElement(BlockInspector, { node: block, defaultOpen: true }));
    const rtl = c.querySelector('[dir="rtl"]');
    expect(rtl).not.toBeNull();
    expect(rtl?.getAttribute("lang")).toBe("ar-EG");
    expect(rtl?.textContent).toContain("مرحباً");
  });

  it("puts dir/lang on a quoted overlay span's own text, keyed by the overlay's side", () => {
    const c = renderEl(createElement(BlockInspector, { node: arabicBlock, defaultOpen: true }));
    const quoted = [...c.querySelectorAll("span")].find((el) => el.textContent?.includes("“"));
    expect(quoted?.getAttribute("dir")).toBe("rtl");
    expect(quoted?.getAttribute("lang")).toBe("ar");
  });
});

describe("StructureView — writing direction", () => {
  function tree(node: ContentNode): ContentTree {
    return { format: "json", stats: {}, root: [node] } as ContentTree;
  }

  it("puts dir/lang on the structure row's own text element", () => {
    const c = renderEl(createElement(StructureView, { tree: tree(arabicBlock) }));
    const row = c.querySelector('[data-testid="structure-row"] span[dir]');
    expect(row?.getAttribute("dir")).toBe("rtl");
    expect(row?.getAttribute("lang")).toBe("ar");
    expect(row?.textContent).toContain("مرحباً");
  });

  it("keys the row's direction off the selected target side, not the source locale", () => {
    const block: ContentNode = {
      ...arabicBlock,
      sourceLocale: "en",
      source: [{ text: "Welcome to Kapi Mart" }],
      targets: { ar: [{ text: "مرحباً بك في كابي مارت" }] },
    };
    const c = renderEl(createElement(StructureView, { tree: tree(block), side: "ar" }));
    const row = c.querySelector('[data-testid="structure-row"] span[dir]');
    expect(row?.getAttribute("dir")).toBe("rtl");
    expect(row?.getAttribute("lang")).toBe("ar");
  });

  it("falls back to the source locale when the selected side has no committed target", () => {
    const block: ContentNode = {
      ...arabicBlock,
      sourceLocale: "ar",
      targets: {},
    };
    const c = renderEl(createElement(StructureView, { tree: tree(block), side: "fr" }));
    const row = c.querySelector('[data-testid="structure-row"] span[dir]');
    // No fr target exists, so the row reads (and must be tagged) as its ar source.
    expect(row?.getAttribute("dir")).toBe("rtl");
    expect(row?.getAttribute("lang")).toBe("ar");
  });
});

describe("LayoutView — writing direction", () => {
  it("puts dir/lang on a layout box's own text label", () => {
    const geoBlock: ContentNode = {
      ...arabicBlock,
      geometry: { page: 1, x: 0, y: 0, w: 100, h: 20, resolution: 500, origin: "top-left" },
    };
    const t: ContentTree = {
      format: "docling",
      stats: {},
      root: [geoBlock],
    } as ContentTree;
    const c = renderEl(createElement(LayoutView, { tree: t }));
    const label = c.querySelector('[data-testid="layout-box"] [dir]');
    expect(label?.getAttribute("dir")).toBe("rtl");
    expect(label?.getAttribute("lang")).toBe("ar");
    expect(label?.textContent).toContain("مرحباً");
  });
});
