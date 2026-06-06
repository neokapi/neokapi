// @vitest-environment jsdom
import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { treeToRenderDoc } from "../renderDoc";
import { overlayStyle, resolveOverlaySpans, segmentText, overlayTypes } from "../overlayHighlight";
import FormatPreview from "../FormatPreview";
import DocumentViewer from "../DocumentViewer";
import FileBrowser from "../FileBrowser";
import type { ContentNode, ContentTree, OverlayView, Run } from "../types";

// ── Fixtures (mirror real `kapi inspect` shapes) ─────────────────────────────

function txt(s: string): Run[] {
  return [{ text: s }];
}

function block(
  id: string,
  type: string,
  source: string,
  extra: Partial<ContentNode> = {},
): ContentNode {
  return { kind: "block", id, type, source: txt(source), ...extra };
}

function layer(
  name: string,
  children: ContentNode[],
  extra: Partial<ContentNode> = {},
): ContentNode {
  return { kind: "layer", id: name, name, children, ...extra };
}

function tree(format: string, root: ContentNode[]): ContentTree {
  return { format, root, stats: { layers: 0, groups: 0, blocks: 0, data: 0, media: 0, runs: 0 } };
}

// Mirror the REAL `labInspectAnnotated` shapes: term overlays carry the matched
// surface + its `target` translation + `domain`; brand violations ride on the
// `qa` overlay type with props.category="brand-vocabulary" + a `replacement`;
// other QA findings are `qa` overlays carrying a `message`.
const termOverlay: OverlayView = {
  type: "term",
  side: "source",
  spans: [
    {
      id: "t1",
      range: { startRun: 0, startOffset: 0, endRun: 0, endOffset: 0 },
      text: "Acme",
      props: { term: "Acme", target: "Acme", domain: "brand" },
    },
  ],
};

const brandOverlay: OverlayView = {
  type: "qa",
  side: "source",
  spans: [
    {
      id: "b1",
      range: { startRun: 0, startOffset: 0, endRun: 0, endOffset: 0 },
      text: "Acme",
      props: {
        category: "brand-vocabulary",
        severity: "critical",
        message: 'Competitor term "Acme" found',
      },
    },
  ],
};

const qaOverlay: OverlayView = {
  type: "qa",
  side: "fr-FR",
  spans: [
    {
      id: "q1",
      range: { startRun: 0, startOffset: 0, endRun: 0, endOffset: 0 },
      text: "Acme",
      props: { category: "doubled-word", message: 'Doubled word: "Acme"' },
    },
  ],
};

// A markdown doc with a source + fr-FR target and term/qa overlays.
const mdTree = tree("markdown", [
  layer("/tmp/guide.md", [
    block("h1", "heading", "Welcome to Acme", {
      targets: { "fr-FR": txt("Bienvenue chez Acme") },
      overlays: [termOverlay, brandOverlay, qaOverlay],
    }),
    block("p1", "", "Acme helps teams ship faster.", {
      targets: { "fr-FR": txt("Acme aide les équipes.") },
    }),
  ]),
]);

const jsonTree = tree("json", [
  layer("/tmp/messages.json", [
    block("greeting", "json:value", "Hello, Acme", { properties: { path: "$.greeting" } }),
    block("bye", "json:value", "Goodbye", { properties: { path: "$.bye" } }),
  ]),
]);

const pdfTree = tree("pdf", [
  layer("/tmp/doc.pdf", [
    block("a", "paragraph", "Page one heading", { properties: { page: "1" } }),
    block("b", "paragraph", "Body of page one", { properties: { page: "1" } }),
    block("c", "paragraph", "Page two heading", { properties: { page: "2" } }),
  ]),
]);

// A format with no recognizable shape and no DOC/LIST hint → sectioned fallback.
const genericTree = tree("acme-custom", [
  layer("/tmp/file.acme", [
    block("s1", "", "First entry", {}),
    {
      kind: "group",
      id: "g1",
      name: "settings",
      children: [block("s2", "", "Nested entry", {})],
    },
  ]),
]);

// ── treeToRenderDoc: targets, overlays, locales ──────────────────────────────

describe("treeToRenderDoc — targets & overlays", () => {
  it("carries per-locale target text and overlays onto render lines", () => {
    const doc = treeToRenderDoc(mdTree);
    expect(doc.kind).toBe("doc");
    expect(doc.locales).toEqual(["fr-FR"]);
    const h = doc.paragraphs!.find((p) => p.id === "h1")!;
    expect(h.text).toBe("Welcome to Acme");
    expect(h.targets).toEqual({ "fr-FR": "Bienvenue chez Acme" });
    expect(h.overlays).toHaveLength(3);
  });

  it("renders an entry list with keys for JSON", () => {
    const doc = treeToRenderDoc(jsonTree);
    expect(doc.kind).toBe("list");
    expect(doc.lines!.map((l) => l.key)).toEqual(["$.greeting", "$.bye"]);
    expect(doc.lines!.map((l) => l.text)).toEqual(["Hello, Acme", "Goodbye"]);
  });

  it("groups PDF blocks into pages by properties.page", () => {
    const doc = treeToRenderDoc(pdfTree);
    expect(doc.kind).toBe("pages");
    expect(doc.pages!.map((p) => p.name)).toEqual(["Page 1", "Page 2"]);
    expect(doc.pages![0].lines.map((l) => l.text)).toEqual([
      "Page one heading",
      "Body of page one",
    ]);
  });

  it("falls back to titled sections for an unknown format with containers", () => {
    const doc = treeToRenderDoc(genericTree);
    expect(doc.kind).toBe("sections");
    expect(doc.sections!.map((s) => s.name)).toEqual(["/tmp/file.acme", "settings"]);
    expect(doc.sections![0].lines.map((l) => l.text)).toEqual(["First entry"]);
    expect(doc.sections![1].lines.map((l) => l.text)).toEqual(["Nested entry"]);
  });
});

// ── overlayHighlight ─────────────────────────────────────────────────────────

describe("overlayHighlight", () => {
  it("resolves source-side spans to char ranges and skips other sides", () => {
    // term + brand are source-side; qa is fr-FR-side and must be skipped here.
    const spans = resolveOverlaySpans(
      [termOverlay, brandOverlay, qaOverlay],
      "source",
      "Welcome to Acme",
    );
    expect(spans).toHaveLength(2);
    expect(spans.map((s) => s.type)).toEqual(["term", "qa"]);
    expect(spans.every((s) => s.start === 11 && s.end === 15)).toBe(true);
    // The term tooltip surfaces its target translation + domain (not raw props).
    const term = spans.find((s) => s.type === "term")!;
    expect(term.tooltip).toContain("Vocabulary");
    expect(term.tooltip).toContain("domain: brand");
  });

  it("resolves target-side spans for the active locale", () => {
    const spans = resolveOverlaySpans([termOverlay, qaOverlay], "fr-FR", "Acme aide");
    expect(spans).toHaveLength(1);
    expect(spans[0].type).toBe("qa");
  });

  it("distinguishes brand-vocabulary qa from plain qa by span props", () => {
    const [brand] = resolveOverlaySpans([brandOverlay], "source", "Welcome to Acme");
    const [plainQa] = resolveOverlaySpans([qaOverlay], "fr-FR", "Acme aide");
    // Brand violation = pink accent + "Brand" label; plain qa = amber "QA".
    expect(brand.style.label).toBe("Brand");
    expect(brand.style.className).toContain("pink");
    expect(plainQa.style.label).toBe("QA");
    expect(plainQa.style.className).toContain("amber");
    expect(brand.style.className).not.toBe(plainQa.style.className);
    // The brand tooltip surfaces the human message.
    expect(brand.tooltip).toContain("Competitor term");
  });

  it("filters by overlay type", () => {
    const spans = resolveOverlaySpans(
      [termOverlay],
      "source",
      "Welcome to Acme",
      new Set(["entity"]),
    );
    expect(spans).toHaveLength(0);
  });

  it("color-codes by type with distinct accents", () => {
    expect(overlayStyle("term").className).not.toBe(overlayStyle("qa").className);
    expect(overlayStyle("term").label).toBe("Vocabulary");
    expect(overlayStyle("qa").label).toBe("QA");
    // A brand-vocabulary qa span resolves to the dedicated brand accent.
    expect(overlayStyle("qa", brandOverlay.spans[0]).label).toBe("Brand");
    expect(overlayStyle("totally-unknown").label).toBe("Totally Unknown");
  });

  it("segments text into plain + overlay runs (innermost wins on overlap)", () => {
    const spans = resolveOverlaySpans([termOverlay], "source", "Welcome to Acme");
    const segs = segmentText("Welcome to Acme", spans);
    expect(segs.map((s) => s.text).join("")).toBe("Welcome to Acme");
    expect(segs.find((s) => s.overlay)?.text).toBe("Acme");
  });

  it("lists distinct overlay types in first-seen order", () => {
    expect(overlayTypes([termOverlay, qaOverlay, termOverlay])).toEqual(["term", "qa"]);
  });
});

// ── Component idle-render (reduced motion, no WASM) ───────────────────────────

// The heading carries a term overlay on "Acme", so its text is split across a
// plain span + a <mark>; assert on the container's combined text content.
function hasText(container: HTMLElement, s: string): boolean {
  return (container.textContent ?? "").includes(s);
}

describe("FormatPreview", () => {
  it("renders the source side of a markdown doc", () => {
    const { container } = render(<FormatPreview tree={mdTree} reducedMotion />);
    expect(hasText(container, "Welcome to Acme")).toBe(true);
  });

  it("renders the chosen target locale's text", () => {
    const { container } = render(<FormatPreview tree={mdTree} side="fr-FR" reducedMotion />);
    expect(hasText(container, "Bienvenue chez Acme")).toBe(true);
  });

  it("highlights overlays with color-coded marks and a type attribute", () => {
    // A doc carrying only the term overlay (the heading in mdTree also carries a
    // brand qa overlay over the same span, which would win the innermost flatten).
    const termOnly = tree("markdown", [
      layer("/tmp/g.md", [block("h1", "heading", "Welcome to Acme", { overlays: [termOverlay] })]),
    ]);
    const { container } = render(<FormatPreview tree={termOnly} reducedMotion annotations />);
    const mark = container.querySelector('mark[data-overlay-type="term"]');
    expect(mark).toBeTruthy();
    expect(mark?.textContent).toBe("Acme");
  });

  it("omits overlay marks when annotations are off", () => {
    const { container } = render(<FormatPreview tree={mdTree} reducedMotion annotations={false} />);
    expect(container.querySelector("mark[data-overlay-type]")).toBeNull();
  });

  it("renders typewriter instantly under reduced motion (full text present)", () => {
    const { container } = render(
      <FormatPreview tree={mdTree} side="fr-FR" transition="typewriter" reducedMotion />,
    );
    expect(hasText(container, "Bienvenue chez Acme")).toBe(true);
  });

  it("renders an xlsx grid from a ContentTree", () => {
    const xlsx = tree("openxml", [
      layer("/tmp/r.xlsx", [
        layer("xl/worksheets/sheet1.xml", [
          block("c1", "cell", "Revenue", { properties: { cell: "A1" } }),
          block("c2", "cell", "100", { properties: { cell: "B1" } }),
        ]),
      ]),
    ]);
    render(<FormatPreview tree={xlsx} reducedMotion />);
    expect(screen.getByText("Revenue")).toBeTruthy();
    expect(screen.getByText("100")).toBeTruthy();
  });
});

describe("DocumentViewer", () => {
  it("shows the preview tab and a download button gated on bytes", () => {
    const bytes = new TextEncoder().encode("# Welcome to Acme");
    const { container } = render(
      <DocumentViewer tree={mdTree} filename="guide.md" bytes={bytes} />,
    );
    expect(hasText(container, "Welcome to Acme")).toBe(true);
    expect(screen.getAllByRole("button", { name: /download/i }).length).toBeGreaterThan(0);
  });

  it("exposes the source↔target locale toggle from targets", () => {
    render(<DocumentViewer tree={mdTree} filename="guide.md" />);
    // The Side select shows "Source" by default (trigger + hidden native select).
    expect(screen.getAllByText("Source").length).toBeGreaterThan(0);
  });
});

describe("FileBrowser", () => {
  it("renders many mixed files with a view toggle", () => {
    render(
      <FileBrowser
        files={[
          { filename: "guide.md", tree: mdTree },
          { filename: "messages.json", tree: jsonTree },
        ]}
      />,
    );
    expect(screen.getAllByText("guide.md").length).toBeGreaterThan(0);
    expect(screen.getAllByText("messages.json").length).toBeGreaterThan(0);
    expect(screen.getByText("2 files")).toBeTruthy();
    // ToggleGroup (type="single") renders its items as radios.
    expect(screen.getByRole("radio", { name: /list view/i })).toBeTruthy();
    expect(screen.getByRole("radio", { name: /grid view/i })).toBeTruthy();
  });
});
