// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeAll, describe, expect, it } from "vitest";
import FilePreview from "../components/preview/FilePreview";
import { highlightSpansByBlock } from "../components/preview/highlights";
import type { PreviewHighlights } from "../components/preview/highlights";
import type { ContentTree } from "../components/preview/types";

// jsdom implements neither of these; the sheet's focus scroll and Radix's
// dismissable layer both reach for them.
beforeAll(() => {
  Element.prototype.scrollIntoView ??= () => {};
  window.HTMLElement.prototype.hasPointerCapture ??= () => false;
});

afterEach(cleanup);

/** A catalog file, which the viewer reads as a key table. */
const catalog: ContentTree = {
  format: "json",
  root: [
    {
      kind: "block",
      id: "b1",
      name: "greeting",
      type: "text",
      translatable: true,
      sourceLocale: "en",
      source: [{ text: "Please utilize the dashboard" }],
      targets: { fr: [{ text: "Veuillez utiliser le tableau de bord" }] },
    },
    {
      kind: "block",
      id: "b2",
      name: "tagline",
      type: "text",
      translatable: true,
      sourceLocale: "en",
      source: [{ text: "Ship faster" }],
    },
    {
      kind: "block",
      id: "b3",
      name: "welcome",
      type: "text",
      translatable: true,
      sourceLocale: "en",
      source: [
        { text: "Hello " },
        { ph: { id: "name", type: "var", data: "{name}", equiv: "name" } },
        { text: "!" },
      ],
      targets: { fr: [{ text: "Bonjour !" }] },
    },
  ],
  stats: { layers: 0, groups: 0, blocks: 3, data: 0, media: 0, runs: 5 },
};

/** The same units as prose, which the viewer lays out as a document. */
const prose: ContentTree = {
  ...catalog,
  format: "markdown",
  root: catalog.root.map((n) => ({ ...n, name: undefined, type: "paragraph" })),
};

const highlights: PreviewHighlights = {
  b1: [
    {
      side: "source",
      anchor: { kind: "range", start: { run: 0, offset: 7 }, end: { run: 0, offset: 14 } },
      tone: "destructive",
      label: 'Forbidden term "utilize" found',
      emphasis: "focus",
    },
  ],
  b2: [
    {
      side: "source",
      anchor: { kind: "block" },
      tone: "warning",
      label: "Tone reads more formal than the brand's register",
      emphasis: "dim",
    },
  ],
  b3: [
    {
      side: "fr",
      anchor: { kind: "block" },
      tone: "destructive",
      label: "The target drops {name}.",
      emphasis: "dim",
    },
    {
      side: "source",
      anchor: { kind: "run", runId: "name" },
      tone: "destructive",
      label: "The target drops {name}.",
      emphasis: "dim",
    },
  ],
};

const base = { onClose: () => {}, filename: "locales/en.json", description: "Read." };

const marks = () =>
  Array.from(document.querySelectorAll<HTMLElement>('mark[data-overlay-type="finding"]'));

describe("highlightSpansByBlock", () => {
  it("locates each block's highlights over the side they name", () => {
    const index = highlightSpansByBlock(catalog, highlights);
    expect([...index.keys()].sort()).toEqual(["b1", "b2", "b3"]);
    const b1 = index.get("b1")!.get("source")!;
    expect(b1).toHaveLength(1);
    expect(b1[0]).toMatchObject({ start: 7, end: 14, type: "finding", emphasis: "focus" });
    expect(b1[0].span.text).toBe("utilize");
    expect(index.get("b2")!.get("source")![0]).toMatchObject({ start: 0, end: 11 });
    expect(index.get("b3")!.get("fr")![0]).toMatchObject({ start: 0, end: 9 });
    expect(index.get("b3")!.get("source")![0]).toMatchObject({ start: 6, end: 6, code: "name" });
  });

  it("is empty with nothing to draw", () => {
    expect(highlightSpansByBlock(catalog, undefined).size).toBe(0);
    expect(highlightSpansByBlock(null, highlights).size).toBe(0);
    expect(highlightSpansByBlock(catalog, { b1: [] }).size).toBe(0);
  });
});

describe("FilePreview highlights", () => {
  it("marks a range on the key table, the one in focus apart from the rest", () => {
    render(<FilePreview {...base} open tree={catalog} highlights={highlights} />);
    const focus = marks().find((m) => m.getAttribute("data-emphasis") === "focus")!;
    expect(focus.textContent).toBe("utilize");
    expect(focus.className).toContain("decoration-destructive");
    const dim = marks().find((m) => m.textContent === "Ship faster")!;
    expect(dim.getAttribute("data-emphasis")).toBe("dim");
    expect(dim.className).toContain("decoration-warning");
  });

  it("draws a target-side highlight over the target column and a run anchor around its chip", () => {
    render(
      <FilePreview
        {...base}
        open
        tree={catalog}
        highlights={highlights}
        viewer={{ defaultSide: "fr" }}
      />,
    );
    expect(marks().some((m) => m.textContent === "Bonjour !")).toBe(true);
    const chip = document.querySelector<HTMLElement>('[data-inline-code="placeholder"]')!;
    expect(chip.closest('mark[data-overlay-type="finding"]')).toBeTruthy();
  });

  it("marks the same spans on the document reading", () => {
    render(<FilePreview {...base} open filename="doc.md" tree={prose} highlights={highlights} />);
    const focus = marks().find((m) => m.getAttribute("data-emphasis") === "focus")!;
    expect(focus.textContent).toBe("utilize");
    expect(marks().some((m) => m.textContent === "Ship faster")).toBe(true);
    expect(screen.getByText(/Please/)).toBeTruthy();
  });

  it("draws the focus row with the host's note and the way back, with no unit named", () => {
    render(
      <FilePreview
        {...base}
        open
        tree={catalog}
        backLabel="Back to checks"
        focusNote={<span>3 findings</span>}
      />,
    );
    const row = document.querySelector<HTMLElement>('[data-slot="file-preview-focus"]')!;
    expect(row).toBeTruthy();
    expect(row.textContent).toContain("Back to checks");
    expect(row.textContent).toContain("3 findings");
    expect(row.textContent).not.toContain("awaiting review");
  });
});
