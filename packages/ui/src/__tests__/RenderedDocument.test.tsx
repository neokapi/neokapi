// @vitest-environment jsdom
import { describe, it, expect } from "vitest";
import { createElement } from "react";
import { createRoot } from "react-dom/client";
import { act } from "react";
import RenderedDocument from "../components/preview/RenderedDocument";
import type { RenderNode } from "../components/preview/types";

function render(node: RenderNode): HTMLDivElement {
  const container = document.createElement("div");
  document.body.appendChild(container);
  act(() => {
    createRoot(container).render(createElement(RenderedDocument, { node }));
  });
  return container;
}

describe("RenderedDocument", () => {
  it("renders inline formatting as semantic markup", () => {
    const node: RenderNode = {
      role: "document",
      children: [
        {
          role: "paragraph",
          runs: [
            { text: "Some " },
            { pcOpen: { id: "1", type: "fmt:bold" } },
            { text: "bold" },
            { pcClose: { id: "1", type: "fmt:bold" } },
            { text: " and a " },
            { pcOpen: { id: "2", type: "link:hyperlink", attrs: { href: "https://x.com" } } },
            { text: "link" },
            { pcClose: { id: "2", type: "link:hyperlink" } },
          ],
        },
      ],
    };
    const c = render(node);
    const strong = c.querySelector("strong");
    expect(strong?.textContent).toBe("bold");
    const a = c.querySelector("a");
    expect(a?.getAttribute("href")).toBe("https://x.com");
    expect(a?.textContent).toBe("link");
  });

  it("renders an image placeholder as <img>", () => {
    const node: RenderNode = {
      role: "document",
      children: [
        {
          role: "paragraph",
          runs: [
            {
              ph: {
                id: "i",
                type: "media:image",
                equiv: "logo",
                attrs: { src: "/l.png", alt: "Logo" },
              },
            },
          ],
        },
      ],
    };
    const img = render(node).querySelector("img");
    expect(img?.getAttribute("src")).toBe("/l.png");
    expect(img?.getAttribute("alt")).toBe("Logo");
  });

  it("reconstructs a table with header cells and spans", () => {
    const cell = (text: string, header = false, extra: Partial<RenderNode> = {}): RenderNode => ({
      role: header ? "table-header" : "table-cell",
      runs: [{ text }],
      header,
      ...extra,
    });
    const node: RenderNode = {
      role: "document",
      children: [
        {
          role: "table",
          children: [
            { role: "table-row", children: [cell("A", true), cell("B", true)] },
            { role: "table-row", children: [cell("1"), cell("2", false, { colSpan: 2 })] },
          ],
        },
      ],
    };
    const c = render(node);
    expect(c.querySelector("table")).not.toBeNull();
    expect(c.querySelectorAll("th")).toHaveLength(2);
    expect(c.querySelector("th")?.textContent).toBe("A");
    const tds = c.querySelectorAll("td");
    expect(tds).toHaveLength(2);
    expect(tds[1].getAttribute("colspan")).toBe("2");
  });

  it("renders headings and ordered lists", () => {
    const node: RenderNode = {
      role: "document",
      children: [
        { role: "heading", level: 2, runs: [{ text: "Title" }] },
        {
          role: "list",
          ordered: true,
          children: [{ role: "list-item", runs: [{ text: "first" }] }],
        },
      ],
    };
    const c = render(node);
    expect(c.querySelector("h2")?.textContent).toBe("Title");
    expect(c.querySelector("ol li")?.textContent).toBe("first");
  });
});
