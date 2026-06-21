// Content-model lessons for ContentLab — each teaches one facet of the content
// model and declares HOW to visualize it: which read-only annotators to run (so
// their stand-off overlays appear), whether to run a command first and inspect
// its output, and which DocumentViewer tab to open. The lab reads everything
// with the real WASM engine; the lesson only chooses the lens.

import type { AnnotateOptions } from "@neokapi/kapi-playground/runtime";
import type { DocumentViewerProps } from "@neokapi/ui-primitives/preview";

/** A DocumentViewer tab (the inspect surface's view). */
export type ContentTab = NonNullable<DocumentViewerProps["defaultTab"]>;

export interface ContentInspectSpec {
  /**
   * Read-only annotators to run before building the tree, so their overlays show
   * (segmentation boundaries, term matches, QA + brand findings). Omitted = a
   * plain structural inspect (anatomy only).
   */
  annotate?: AnnotateOptions;
  /**
   * Run a command first, then visualize its OUTPUT instead of the input. argv
   * uses `{in}` / `{out}` placeholders (absolute /project paths are substituted).
   * `diff: true` shows the written output diffed against the input (round-trip);
   * otherwise the output is inspected for its content model (e.g. targets).
   */
  run?: { argv: string[]; diff?: boolean };
  /** DocumentViewer tab to open first. */
  tab?: ContentTab;
}

export interface ContentLesson {
  id: string;
  label: string;
  /** One-line teaching goal shown under the picker. */
  description: string;
  /** Sample preselected for this lesson (id from SAMPLES). */
  sampleId: string;
  spec: ContentInspectSpec;
}

export const CONTENT_LESSONS: ContentLesson[] = [
  {
    id: "anatomy",
    label: "Anatomy — layers, blocks, runs",
    description:
      "A reader turns the file into the content model: nested Layers and Groups holding Blocks whose text is a flat sequence of Runs. Open a Block to read its runs — an HTML <strong> becomes a paired inline code; a JSON {name} stays literal text. That is format-awareness.",
    sampleId: "page-html",
    spec: { tab: "blocks" },
  },
  {
    id: "segmentation",
    label: "Segmentation overlay",
    description:
      "Sentence segmentation is a stand-off overlay, not a structural split: boundaries are anchored over the runs (dotted markers in the run sequence) while the text itself is never cut apart. Open a Block to see the segmentation overlay; the runs are untouched.",
    sampleId: "support-reply",
    spec: { annotate: { segment: true }, tab: "blocks" },
  },
  {
    id: "annotations",
    label: "Terms & QA overlays",
    description:
      "Tools communicate by attaching stand-off state, never by rewriting text: terminology matches and rule-based QA ride as overlays on the source. The Preview highlights each span in place; open a Block to read the overlay's properties.",
    sampleId: "support-reply",
    spec: { annotate: { term: true, qa: true, brand: true }, tab: "preview" },
  },
  {
    id: "bilingual",
    label: "Source ↔ target",
    description:
      "A target is a first-class, variant-keyed record on the Block — not a second copy of the file. This bilingual XLIFF already carries a French target for every segment: the Preview's source↔target toggle swaps between them, and each Block lists its target alongside its source.",
    sampleId: "greeting-bilingual-xliff",
    spec: { tab: "preview" },
  },
  {
    id: "structure",
    label: "Document structure",
    description:
      "Beyond text, a Block carries its semantic role and reading order (heading, paragraph, list-item, table-cell…). The Structure tab outlines the document logically — the same model whatever format stored it.",
    sampleId: "report-doclang",
    spec: { tab: "structure" },
  },
  {
    id: "roundtrip",
    label: "Round-trip — skeleton preserved",
    description:
      "The writer splices block text back into the original file's skeleton. Pseudo-translate every block, then diff the output against the input: only the block text changed — structure, markup and key order are byte-for-byte intact. That is the round-trip guarantee.",
    sampleId: "messages-json",
    spec: { run: { argv: ["pseudo-translate", "{in}", "-o", "{out}"], diff: true }, tab: "raw" },
  },
];
