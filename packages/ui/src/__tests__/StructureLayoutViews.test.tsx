// @vitest-environment jsdom
import { describe, it, expect, afterEach } from "vitest";
import { createElement } from "react";
import { createRoot } from "react-dom/client";
import { act } from "react";
import StructureView from "../components/preview/StructureView";
import LayoutView from "../components/preview/LayoutView";
import { roleStyle } from "../components/preview/roleStyle";
import type { ContentNode, ContentTree } from "../components/preview/types";

function renderToContainer(el: React.ReactElement): HTMLDivElement {
  const container = document.createElement("div");
  document.body.appendChild(container);
  act(() => {
    createRoot(container).render(el);
  });
  return container;
}

function block(id: string, text: string, extra: Partial<ContentNode> = {}): ContentNode {
  return { kind: "block", id, source: [{ text }], ...extra };
}

function tree(root: ContentNode[]): ContentTree {
  return {
    format: "docling",
    root,
    stats: { layers: 0, groups: 0, blocks: 0, data: 0, media: 0, runs: 0 },
  };
}

afterEach(() => {
  while (document.body.firstChild) document.body.removeChild(document.body.firstChild);
});

describe("roleStyle", () => {
  it("carries heading level into the label", () => {
    expect(roleStyle("heading", 2).label).toBe("Heading 2");
    expect(roleStyle("title", 0).label).toBe("Title");
  });
  it("humanizes an unknown role", () => {
    expect(roleStyle("page-header").label).toBe("Page header");
    expect(roleStyle("weird_thing").label).toBe("Weird thing");
  });
  it("falls back for an empty role", () => {
    expect(roleStyle(undefined).label).toBe("Block");
  });
});

describe("StructureView", () => {
  it("outlines blocks with role labels, order, and text", () => {
    const t = tree([
      block("b1", "Annual Report", { structure: { role: "title" } }),
      block("b2", "Overview", { structure: { role: "heading", level: 2 } }),
      block("b3", "Body text.", { structure: { role: "paragraph" } }),
    ]);
    const c = renderToContainer(createElement(StructureView, { tree: t }));
    const rows = c.querySelectorAll('[data-testid="structure-row"]');
    expect(rows.length).toBe(3);
    expect(c.textContent).toContain("Title");
    expect(c.textContent).toContain("Heading 2");
    expect(c.textContent).toContain("Annual Report");
    expect(c.textContent).toContain("Overview");
    // reading-order numbers
    expect(c.textContent).toContain("1");
    expect(c.textContent).toContain("3");
  });

  it("marks furniture blocks", () => {
    const t = tree([
      block("b1", "Confidential", { structure: { role: "page-header", layer: "furniture" } }),
    ]);
    const c = renderToContainer(createElement(StructureView, { tree: t }));
    expect(c.textContent).toContain("furniture");
  });

  it("indents blocks nested inside groups", () => {
    const t = tree([
      {
        kind: "group",
        id: "g1",
        children: [
          block("li1", "First", { structure: { role: "list-item" } }),
          block("li2", "Second", { structure: { role: "list-item" } }),
        ],
      },
    ]);
    const c = renderToContainer(createElement(StructureView, { tree: t }));
    const rows = c.querySelectorAll<HTMLElement>('[data-testid="structure-row"]');
    expect(rows.length).toBe(2);
    // group children are indented (paddingLeft > 0)
    expect(rows[0].style.paddingLeft).not.toBe("0rem");
  });

  it("shows the target text when a side is selected", () => {
    const t = tree([
      {
        kind: "block",
        id: "b1",
        source: [{ text: "Hello" }],
        targets: { "fr-FR": [{ text: "Bonjour" }] },
        structure: { role: "paragraph" },
      },
    ]);
    const c = renderToContainer(createElement(StructureView, { tree: t, side: "fr-FR" }));
    expect(c.textContent).toContain("Bonjour");
    expect(c.textContent).not.toContain("Hello");
  });
});

describe("LayoutView", () => {
  const geoTree = () =>
    tree([
      block("b1", "Title", {
        structure: { role: "title" },
        geometry: { page: 1, x: 72, y: 60, w: 428, h: 32, resolution: 512, origin: "top-left" },
      }),
      block("b2", "Body", {
        structure: { role: "paragraph" },
        geometry: { page: 1, x: 72, y: 120, w: 400, h: 200, resolution: 512, origin: "top-left" },
      }),
      block("b3", "Next page", {
        structure: { role: "paragraph" },
        geometry: { page: 2, x: 72, y: 60, w: 400, h: 40, resolution: 512, origin: "top-left" },
      }),
    ]);

  it("renders one canvas per page with positioned boxes", () => {
    const c = renderToContainer(createElement(LayoutView, { tree: geoTree() }));
    const pages = c.querySelectorAll('[data-testid="layout-page"]');
    expect(pages.length).toBe(2);
    const boxes = c.querySelectorAll<HTMLElement>('[data-testid="layout-box"]');
    expect(boxes.length).toBe(3);
    // first box positioned from its bbox: left = 72/512 ≈ 14%.
    expect(boxes[0].style.left).toMatch(/^14\./);
    expect(boxes[0].style.width).toMatch(/^83\./); // 428/512 ≈ 83.6%
    expect(c.textContent).toContain("Title");
  });

  it("flips Y for bottom-left origin", () => {
    const t = tree([
      block("b1", "Bottom box", {
        structure: { role: "paragraph" },
        geometry: { page: 1, x: 0, y: 0, w: 100, h: 50, resolution: 500, origin: "bottom-left" },
      }),
    ]);
    const c = renderToContainer(createElement(LayoutView, { tree: t }));
    const box = c.querySelector<HTMLElement>('[data-testid="layout-box"]')!;
    // bottom-left y=0,h=50 in a 500 grid → top = (500-0-50)/500 = 90%.
    expect(box.style.top).toMatch(/^90/);
  });

  it("reports an empty state when no geometry is present", () => {
    const t = tree([block("b1", "no geo", { structure: { role: "paragraph" } })]);
    const c = renderToContainer(createElement(LayoutView, { tree: t }));
    expect(c.querySelectorAll('[data-testid="layout-page"]').length).toBe(0);
    expect(c.textContent).toContain("No page geometry");
  });

  it("notes unplaced blocks", () => {
    const t = tree([
      block("b1", "placed", {
        geometry: { page: 1, x: 0, y: 0, w: 10, h: 10, resolution: 100 },
      }),
      block("b2", "unplaced"),
    ]);
    const c = renderToContainer(createElement(LayoutView, { tree: t }));
    expect(c.textContent).toContain("1 block without geometry");
  });
});
