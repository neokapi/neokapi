// @vitest-environment jsdom
//
// Inline codes survive the flattening into a RenderDoc.
//
// A placeholder or a paired tag carries no literal text, so concatenating a
// block's runs leaves it with nowhere to land: a preview built that way showed
// "Your credits reset on ." beside a target that showed the variable — the
// source read as if it had no placeholder at all. The codes are positioned
// alongside the text instead and rendered as chips, on whichever side is read.
import { describe, it, expect } from "vitest";
import { createElement, act } from "react";
import { createRoot } from "react-dom/client";

import FormatPreview from "../components/preview/FormatPreview";
import { runsCodes, treeToRenderDoc } from "../components/preview/renderDoc";
import type { ContentTree } from "../components/preview/types";

function render(el: React.ReactElement): HTMLDivElement {
  const container = document.createElement("div");
  document.body.appendChild(container);
  act(() => {
    createRoot(container).render(el);
  });
  return container;
}

function chips(container: HTMLElement): string[] {
  return [...container.querySelectorAll("[data-inline-code]")].map((el) => el.textContent ?? "");
}

const resetDate = {
  ph: { id: "1", type: "code:variable", data: "{{.ResetDate}}", equiv: "{{.ResetDate}}" },
};

/** The credits-exhausted email block: text · variable · text. */
const tree: ContentTree = {
  format: "json",
  stats: { layers: 0, groups: 0, blocks: 1, data: 0, media: 0, runs: 3 },
  root: [
    {
      kind: "block",
      id: "b1",
      translatable: true,
      source: [{ text: "Your credits reset on " }, resetDate, { text: ". Upgrade any time." }],
      targets: {
        nb: [{ text: "Kredittene tilbakestilles " }, resetDate, { text: ". Oppgrader når som." }],
      },
    },
  ],
};

describe("runsCodes", () => {
  it("positions each inline code in the text the runs flatten to", () => {
    expect(runsCodes(tree.root[0].source)).toEqual([
      {
        offset: "Your credits reset on ".length,
        span: {
          span_type: "placeholder",
          type: "code:variable",
          sub_type: undefined,
          id: "1",
          data: "{{.ResetDate}}",
          equiv_text: "{{.ResetDate}}",
          display_text: undefined,
        },
      },
    ]);
  });

  it("gives a paired code an offset for each half", () => {
    const codes = runsCodes([
      { pcOpen: { id: "1", type: "fmt:bold", data: "<b>", equiv: "<b>" } },
      { text: "now" },
      { pcClose: { id: "1", type: "fmt:bold", data: "</b>", equiv: "</b>" } },
    ]);
    expect(codes.map((c) => [c.offset, c.span.span_type])).toEqual([
      [0, "opening"],
      [3, "closing"],
    ]);
  });

  it("carries the codes of every side onto the render line", () => {
    const line = treeToRenderDoc(tree).lines?.[0];
    expect(line?.codes?.[0].offset).toBe("Your credits reset on ".length);
    expect(line?.targetCodes?.nb[0].offset).toBe("Kredittene tilbakestilles ".length);
  });
});

describe("FormatPreview — inline codes", () => {
  it("renders the source's placeholder as a chip, in place", () => {
    const c = render(createElement(FormatPreview, { tree }));

    expect(chips(c)).toEqual(["var"]);
    expect(c.textContent).toContain("Your credits reset on ");
    expect(c.textContent).toContain(". Upgrade any time.");
  });

  it("renders the target's placeholder too, so the two sides read alike", () => {
    const c = render(createElement(FormatPreview, { tree, side: "nb" }));

    expect(chips(c)).toEqual(["var"]);
    expect(c.textContent).toContain("Kredittene tilbakestilles ");
  });

  it("shows the source's codes for a target that has no translation yet", () => {
    const untranslated: ContentTree = {
      ...tree,
      root: [{ ...tree.root[0], targets: {} }],
    };
    const c = render(createElement(FormatPreview, { tree: untranslated, side: "nb" }));

    expect(chips(c)).toEqual(["var"]);
  });

  it("reads a plural block as one form, marked as one, rather than as nothing", () => {
    const plural: ContentTree = {
      format: "json",
      stats: { layers: 0, groups: 0, blocks: 1, data: 0, media: 0, runs: 1 },
      root: [
        {
          kind: "block",
          id: "b1",
          translatable: true,
          source: [
            {
              plural: {
                pivot: "count",
                forms: { one: [{ text: "1 credit left" }], other: [{ text: "n credits left" }] },
              },
            },
          ],
        },
      ],
    };
    const c = render(createElement(FormatPreview, { tree: plural }));

    // The whole block used to flatten to "" — a blank line where the content is.
    expect(c.textContent).toContain("n credits left");
    expect(chips(c)).toEqual(["plural"]);
  });

  it("splits an overlay around a code that falls inside it", () => {
    const annotated: ContentTree = {
      ...tree,
      root: [
        {
          ...tree.root[0],
          overlays: [
            {
              type: "term",
              side: "source",
              spans: [
                {
                  id: "t1",
                  range: {
                    kind: "range",
                    start: { run: 0, offset: 13 },
                    end: { run: 2, offset: 6 },
                  },
                  text: "reset on . Upgrade",
                },
              ],
            },
          ],
        },
      ],
    };
    const c = render(createElement(FormatPreview, { tree: annotated, reducedMotion: true }));

    const marks = [...c.querySelectorAll("[data-overlay-type='term']")].map((m) => m.textContent);
    expect(marks).toEqual(["reset on ", ". Upgrade"]);
    expect(chips(c)).toEqual(["var"]);
  });
});
