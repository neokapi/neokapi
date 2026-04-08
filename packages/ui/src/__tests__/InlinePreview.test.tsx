// @vitest-environment jsdom
import { describe, it, expect, afterEach } from "vitest";
import { createElement } from "react";
import { createRoot } from "react-dom/client";
import { act } from "react";
import { InlinePreview } from "../components/editor/InlinePreview";
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

describe("InlinePreview", () => {
  afterEach(() => {
    while (document.body.firstChild) {
      document.body.removeChild(document.body.firstChild);
    }
  });

  it("renders nothing when codedText is empty", () => {
    const c = renderToContainer(createElement(InlinePreview, { codedText: "", spans: [] }));
    expect(c.textContent).toBe("");
  });

  it("renders Preview label", () => {
    const c = renderToContainer(
      createElement(InlinePreview, { codedText: "Hello world", spans: [] }),
    );
    expect(c.textContent).toContain("Preview:");
  });

  it("renders formatted HTML preview for coded text with spans", () => {
    const codedText = "Click \uE001here\uE002 to continue";
    const c = renderToContainer(
      createElement(InlinePreview, { codedText, spans: [boldOpen, boldClose] }),
    );
    const boldEl = c.querySelector("b");
    expect(boldEl).not.toBeNull();
    expect(boldEl?.textContent).toBe("here");
  });
});
