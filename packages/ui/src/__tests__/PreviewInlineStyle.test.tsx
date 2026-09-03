// @vitest-environment jsdom
//
// The structured preview reads a block's inline codes two ways: a presentational
// code (bold, italic, monospace) renders as its real element, and an opaque code
// (placeholder, link, break) renders as a chip. This is what stopped a `<code>`
// pair from reading as `[CODE]…/code` around plain text, and what keeps a link's
// open/close chips symmetric.
import { describe, it, expect } from "vitest";
import React, { act } from "react";
import { createRoot } from "react-dom/client";

import { weaveInline } from "../components/preview/inlineContent";
import KeyedTable from "../components/preview/KeyedTable";
import type { InlineCode } from "../components/preview/renderDoc";
import type { TextSegment } from "../components/preview/overlayHighlight";
import type { ContentTree, Run } from "../components/preview/types";
import type { SpanInfo } from "../types/span";

function render(el: React.ReactElement): HTMLDivElement {
  const c = document.createElement("div");
  document.body.appendChild(c);
  act(() => {
    createRoot(c).render(el);
  });
  return c;
}

const plain = (_seg: TextSegment, text: string, key: string) => (
  <React.Fragment key={key}>{text}</React.Fragment>
);

/** Weave a single stretch of text with the given codes (no overlays). */
function weave(text: string, codes: InlineCode[]): HTMLDivElement {
  return render(<>{weaveInline([{ text }], codes, text.length, plain)}</>);
}

const open = (type: string): SpanInfo => ({ span_type: "opening", type, id: "1", data: "" });
const close = (type: string): SpanInfo => ({ span_type: "closing", type, id: "1", data: "" });
const ph = (type: string, data = ""): SpanInfo => ({
  span_type: "placeholder",
  type,
  id: "1",
  data,
});

describe("weaveInline: presentational codes render as their real element", () => {
  it("renders a bold pair as <strong>, not chips", () => {
    const c = weave("Sale", [
      { offset: 0, span: open("fmt:bold") },
      { offset: 4, span: close("fmt:bold") },
    ]);
    expect(c.querySelector("strong")?.textContent).toBe("Sale");
    expect(c.querySelectorAll("[data-inline-code]")).toHaveLength(0);
  });

  it("renders a code pair as monospace <code>, not [CODE]…/code chips", () => {
    const c = weave("order.created", [
      { offset: 0, span: open("fmt:code") },
      { offset: 13, span: close("fmt:code") },
    ]);
    expect(c.querySelector("code")?.textContent).toBe("order.created");
    expect(c.querySelectorAll("[data-inline-code]")).toHaveLength(0);
  });

  it("nests one presentational pair inside another", () => {
    // "big code" bold, with "code" also monospace.
    const c = weave("big code", [
      { offset: 0, span: open("fmt:bold") },
      { offset: 4, span: open("fmt:code") },
      { offset: 8, span: close("fmt:code") },
      { offset: 8, span: close("fmt:bold") },
    ]);
    expect(c.querySelector("strong code")?.textContent).toBe("code");
  });
});

describe("weaveInline: opaque codes stay chips", () => {
  it("renders a placeholder as a chip", () => {
    const c = weave("Reset soon", [{ offset: 10, span: ph("code:variable", "{date}") }]);
    expect(c.querySelector('[data-inline-code="placeholder"]')).not.toBeNull();
  });

  it("gives a link pair a symmetric open/close chip label", () => {
    const c = weave("here", [
      { offset: 0, span: open("link:hyperlink") },
      { offset: 4, span: close("link:hyperlink") },
    ]);
    expect(c.querySelector('[data-inline-code="opening"]')?.textContent).toBe("[A]");
    expect(c.querySelector('[data-inline-code="closing"]')?.textContent).toBe("[/A]");
  });

  it("ends the line after a break", () => {
    const c = weave("AB", [{ offset: 1, span: ph("struct:break", "<br/>") }]);
    expect(c.querySelectorAll("br")).toHaveLength(1);
    expect(c.querySelector("[data-inline-code]")).not.toBeNull();
  });
});

// ── Overlays compose with the now-styled text ────────────────────────────────

describe("weaveInline: overlays layer over styled text", () => {
  it("renders an overlay over text inside a <strong>", () => {
    // <b>Big Sale now</b>, with "Sale" (offset 4..8) overlaid.
    const runs: Run[] = [
      { pcOpen: { id: "1", type: "fmt:bold", data: "<b>" } },
      { text: "Big Sale now" },
      { pcClose: { id: "1", type: "fmt:bold", data: "</b>" } },
    ];
    // The overlay is anchored to the text run (index 1), offsets within it.
    const tree = {
      format: "json",
      stats: { layers: 0, groups: 0, blocks: 1, data: 0, media: 0, runs: 3 },
      root: [
        {
          kind: "block",
          id: "b1",
          translatable: true,
          source: runs,
          overlays: [
            {
              type: "term",
              side: "source",
              spans: [
                {
                  id: "t1",
                  range: {
                    kind: "range",
                    start: { run: 1, offset: 4 },
                    end: { run: 1, offset: 8 },
                  },
                  text: "Sale",
                },
              ],
            },
          ],
        },
      ],
    } as unknown as ContentTree;
    const c = render(<KeyedTable tree={tree} />);
    // The bold renders, and the term mark sits inside it.
    expect(c.querySelector("strong [data-overlay-type='term']")?.textContent).toBe("Sale");
  });

  it("renders a code (bold pair) that falls inside an overlay", () => {
    // "Big Sale now" overlaid whole, with "Sale" (offset 4..8) bold.
    const runs: Run[] = [
      { text: "Big " },
      { pcOpen: { id: "1", type: "fmt:bold", data: "<b>" } },
      { text: "Sale" },
      { pcClose: { id: "1", type: "fmt:bold", data: "</b>" } },
      { text: " now" },
    ];
    const tree = {
      format: "json",
      stats: { layers: 0, groups: 0, blocks: 1, data: 0, media: 0, runs: 5 },
      root: [
        {
          kind: "block",
          id: "b1",
          translatable: true,
          source: runs,
          overlays: [
            {
              type: "term",
              side: "source",
              spans: [
                {
                  id: "t1",
                  range: {
                    kind: "range",
                    start: { run: 0, offset: 0 },
                    end: { run: 4, offset: 4 },
                  },
                  text: "Big Sale now",
                },
              ],
            },
          ],
        },
      ],
    } as unknown as ContentTree;
    const c = render(<KeyedTable tree={tree} />);
    // The bold word renders, and the overlay is preserved across the code
    // boundary (a mark on either side of the bold, and one within it).
    expect(c.querySelector("strong")?.textContent).toBe("Sale");
    expect(c.querySelectorAll("[data-overlay-type='term']").length).toBeGreaterThanOrEqual(2);
  });
});
