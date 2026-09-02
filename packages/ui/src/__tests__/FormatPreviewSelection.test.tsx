// @vitest-environment jsdom
//
// FormatPreview's block affordances: every block carries its id, a host may
// decorate it, and — when the host listens — the document itself is how a block
// is selected, by pointer or by keyboard.
import { describe, it, expect, vi } from "vitest";
import { createElement, act } from "react";
import { createRoot } from "react-dom/client";

import FormatPreview from "../components/preview/FormatPreview";
import type { ContentTree } from "../components/preview/types";

function render(el: React.ReactElement): HTMLDivElement {
  const container = document.createElement("div");
  document.body.appendChild(container);
  act(() => {
    createRoot(container).render(el);
  });
  return container;
}

function blockEl(container: HTMLElement, id: string): HTMLElement {
  const el = container.querySelector<HTMLElement>(`[data-block-id="${id}"]`);
  if (!el) throw new Error(`no element for block ${id}`);
  return el;
}

function click(el: HTMLElement): void {
  act(() => {
    el.dispatchEvent(new MouseEvent("click", { bubbles: true }));
  });
}

function press(el: HTMLElement, key: string): void {
  act(() => {
    el.dispatchEvent(new KeyboardEvent("keydown", { key, bubbles: true }));
  });
}

const tree: ContentTree = {
  format: "json",
  stats: { layers: 0, groups: 0, blocks: 2, data: 0, media: 0, runs: 2 },
  root: [
    { kind: "block", id: "b1", translatable: true, source: [{ text: "Hello world" }] },
    { kind: "block", id: "b2", translatable: true, source: [{ text: "Goodbye now" }] },
  ],
};

describe("FormatPreview — block identity and decoration", () => {
  it("puts every block's id on its element, with no affordance by default", () => {
    const c = render(createElement(FormatPreview, { tree }));

    expect(blockEl(c, "b1").textContent).toContain("Hello world");
    expect(blockEl(c, "b1").getAttribute("role")).toBeNull();
    expect(blockEl(c, "b1").getAttribute("tabindex")).toBeNull();
  });

  it("applies the host's class name and data markers to the block element", () => {
    const c = render(
      createElement(FormatPreview, {
        tree,
        blockAttrs: (id: string) => ({ className: `host-${id}`, "data-status": "reviewed" }),
      }),
    );

    expect(blockEl(c, "b2").className).toContain("host-b2");
    expect(blockEl(c, "b2").getAttribute("data-status")).toBe("reviewed");
  });
});

describe("FormatPreview — selecting a block from the document", () => {
  it("reports a clicked block and marks the selected one", () => {
    const onSelectBlock = vi.fn();
    const c = render(createElement(FormatPreview, { tree, onSelectBlock, selectedBlockId: "b1" }));

    expect(blockEl(c, "b1").getAttribute("aria-current")).toBe("true");
    expect(blockEl(c, "b2").getAttribute("aria-current")).toBeNull();

    click(blockEl(c, "b2"));
    expect(onSelectBlock).toHaveBeenCalledWith("b2");
  });

  it("is a focusable button, activated by Enter or Space", () => {
    const onSelectBlock = vi.fn();
    const c = render(createElement(FormatPreview, { tree, onSelectBlock }));

    expect(blockEl(c, "b1").getAttribute("role")).toBe("button");
    expect(blockEl(c, "b1").getAttribute("tabindex")).toBe("0");

    press(blockEl(c, "b1"), "Enter");
    expect(onSelectBlock).toHaveBeenCalledWith("b1");

    press(blockEl(c, "b2"), " ");
    expect(onSelectBlock).toHaveBeenLastCalledWith("b2");
  });

  it("leaves a text selection alone — highlighting a phrase is not a click", () => {
    const onSelectBlock = vi.fn();
    const c = render(createElement(FormatPreview, { tree, onSelectBlock }));

    // Stand in for a drag-select that ends inside the block: the browser
    // reports a live, non-collapsed selection when the click arrives.
    const range = document.createRange();
    range.selectNodeContents(blockEl(c, "b1"));
    const selection = window.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);

    click(blockEl(c, "b1"));
    expect(onSelectBlock).not.toHaveBeenCalled();

    selection?.removeAllRanges();
    click(blockEl(c, "b1"));
    expect(onSelectBlock).toHaveBeenCalledWith("b1");
  });
});

describe("FormatPreview — the target reading", () => {
  it("renders a locale's target text in place of the source", () => {
    const withTarget: ContentTree = {
      ...tree,
      root: [
        {
          kind: "block",
          id: "b1",
          translatable: true,
          source: [{ text: "Hello world" }],
          targets: { "fr-FR": [{ text: "Bonjour le monde" }] },
        },
      ],
    };
    // The document reading swaps the source for the target. (The keyed reading
    // puts them in adjacent columns instead; see KeyedTable.)
    const c = render(
      createElement(FormatPreview, { tree: withTarget, side: "fr-FR", keyed: false }),
    );

    expect(c.textContent).toContain("Bonjour le monde");
    expect(c.textContent).not.toContain("Hello world");
  });
});
