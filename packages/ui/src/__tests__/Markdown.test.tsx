// @vitest-environment jsdom
import { describe, it, expect } from "vitest";
import { createElement, act } from "react";
import { createRoot } from "react-dom/client";
import { Markdown } from "../components/ui/markdown";

function render(el: React.ReactElement): HTMLDivElement {
  const container = document.createElement("div");
  document.body.appendChild(container);
  act(() => {
    createRoot(container).render(el);
  });
  return container;
}

describe("Markdown", () => {
  it("renders emphasis, inline code and links instead of raw markup", () => {
    const c = render(
      createElement(Markdown, {
        children: "Rewrites the **source** with `redact.rules` — see [docs](https://neokapi.org).",
      }),
    );
    // The literal markup characters must NOT survive to the text content.
    expect(c.textContent).not.toContain("**");
    expect(c.textContent).not.toContain("`redact.rules`");
    // Emphasis, code and links become real elements.
    expect(c.querySelector("strong")?.textContent).toBe("source");
    expect(c.querySelector("code")?.textContent).toBe("redact.rules");
    const a = c.querySelector("a");
    expect(a?.getAttribute("href")).toBe("https://neokapi.org");
    expect(a?.getAttribute("target")).toBe("_blank");
    expect(a?.getAttribute("rel")).toContain("noopener");
  });

  it("renders GFM tables via remark-gfm", () => {
    const c = render(
      createElement(Markdown, {
        children: "| A | B |\n| - | - |\n| 1 | 2 |",
      }),
    );
    expect(c.querySelector("table")).not.toBeNull();
    expect(c.querySelectorAll("td").length).toBe(2);
  });

  it("collapses paragraphs to inline flow in inline mode", () => {
    const c = render(
      createElement(Markdown, {
        inline: true,
        children: "a **bold** word",
      }),
    );
    // No block <p> wrapper in inline mode.
    expect(c.querySelector("p")).toBeNull();
    expect(c.querySelector("strong")?.textContent).toBe("bold");
  });

  it("renders nothing for empty or whitespace input", () => {
    expect(render(createElement(Markdown, { children: "" })).textContent).toBe("");
    expect(render(createElement(Markdown, { children: "   \n  " })).textContent).toBe("");
    expect(render(createElement(Markdown, { children: null })).textContent).toBe("");
  });
});
