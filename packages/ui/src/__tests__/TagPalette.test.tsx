// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from "vitest";
import { createElement } from "react";
import { createRoot } from "react-dom/client";
import { act } from "react";
import { TagPalette } from "../components/editor/TagPalette";
import type { SpanInfo } from "../types/span";

const boldOpen: SpanInfo = { span_type: "opening", type: "fmt:bold", id: "1", data: "<b>" };
const boldClose: SpanInfo = { span_type: "closing", type: "fmt:bold", id: "1", data: "</b>" };

function renderToContainer(el: React.ReactElement): HTMLDivElement {
  const container = document.createElement("div");
  document.body.appendChild(container);
  act(() => {
    createRoot(container).render(el);
  });
  return container;
}

describe("TagPalette", () => {
  afterEach(() => {
    // Clean up DOM between tests.
    while (document.body.firstChild) {
      document.body.removeChild(document.body.firstChild);
    }
  });

  it("renders nothing when sourceSpans is empty", () => {
    const c = renderToContainer(createElement(TagPalette, { sourceSpans: [], onInsert: vi.fn() }));
    expect(c.textContent).toBe("");
  });

  it("renders tag buttons for source spans", () => {
    const c = renderToContainer(
      createElement(TagPalette, { sourceSpans: [boldOpen, boldClose], onInsert: vi.fn() }),
    );
    expect(c.querySelector('[data-testid="tag-palette-0"]')).not.toBeNull();
    expect(c.querySelector('[data-testid="tag-palette-1"]')).not.toBeNull();
  });

  it("calls onInsert when a tag is clicked", () => {
    const onInsert = vi.fn();
    const c = renderToContainer(
      createElement(TagPalette, { sourceSpans: [boldOpen, boldClose], onInsert }),
    );
    const btn = c.querySelector('[data-testid="tag-palette-0"]') as HTMLButtonElement;
    act(() => {
      btn.click();
    });
    expect(onInsert).toHaveBeenCalledWith(boldOpen);
  });

  it("shows Tags: label", () => {
    const c = renderToContainer(
      createElement(TagPalette, { sourceSpans: [boldOpen, boldClose], onInsert: vi.fn() }),
    );
    expect(c.textContent).toContain("Tags:");
  });
});
