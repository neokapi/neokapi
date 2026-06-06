import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import FormatPreview from "../FormatPreview";
import type { PreviewSide } from "../FormatPreview";
import type { TransitionEffect } from "../useTextTransition";
import {
  docxTree,
  genericTree,
  jsonTree,
  mdTree,
  pdfTree,
  pptxTree,
  xlsxTree,
} from "./previewFixtures";

// FormatPreview renders ANY neokapi format from a ContentTree: the structural
// shape (slides / sheet / page / pages / list / sections) is decided by the
// engine's layer tree, not the file name. These stories cover every shape, plus
// overlay highlighting, the source↔target toggle, and the transition effects.

const meta: Meta<typeof FormatPreview> = {
  title: "Lab/PreviewKit/FormatPreview",
  component: FormatPreview,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof FormatPreview>;

// ── One per structural shape ─────────────────────────────────────────────────
export const Slides: Story = {
  render: () => <FormatPreview tree={pptxTree} className="max-w-md" />,
};
export const Sheet: Story = {
  render: () => <FormatPreview tree={xlsxTree} className="max-w-lg" />,
};
export const DocxPage: Story = {
  name: "Doc (docx)",
  render: () => <FormatPreview tree={docxTree} className="max-w-lg" />,
};
export const Markdown: Story = {
  render: () => <FormatPreview tree={mdTree} className="max-w-lg" />,
};
export const JsonList: Story = {
  name: "Entry list (json)",
  render: () => <FormatPreview tree={jsonTree} className="max-w-lg" />,
};
export const PdfPages: Story = {
  name: "Paged (pdf)",
  render: () => <FormatPreview tree={pdfTree} className="max-w-lg" />,
};
export const GenericSections: Story = {
  name: "Generic fallback (sections)",
  render: () => <FormatPreview tree={genericTree} className="max-w-lg" />,
};

// ── Annotation highlighting on / off ─────────────────────────────────────────
export const Annotations: Story = {
  name: "Annotations (overlays)",
  render: () => (
    <div className="flex flex-wrap gap-6">
      <div className="max-w-sm flex-1">
        <p className="mb-2 text-sm font-semibold text-muted-foreground">Annotations on</p>
        <FormatPreview tree={mdTree} annotations />
      </div>
      <div className="max-w-sm flex-1">
        <p className="mb-2 text-sm font-semibold text-muted-foreground">Annotations off</p>
        <FormatPreview tree={mdTree} annotations={false} />
      </div>
    </div>
  ),
};

// ── Source ↔ target ──────────────────────────────────────────────────────────
export const SourceVsTarget: Story = {
  name: "Source vs target (EN → FR)",
  render: () => (
    <div className="flex flex-wrap gap-6">
      <div className="max-w-md flex-1">
        <p className="mb-2 text-sm font-semibold text-muted-foreground">Source</p>
        <FormatPreview tree={pptxTree} side="source" />
      </div>
      <div className="max-w-md flex-1">
        <p className="mb-2 text-sm font-semibold text-muted-foreground">Target · fr-FR</p>
        <FormatPreview tree={pptxTree} side="fr-FR" />
      </div>
    </div>
  ),
};

// ── Typewriter transition (interactive) ──────────────────────────────────────
function TransitionDemo({ effect }: { effect: TransitionEffect }) {
  const [side, setSide] = useState<PreviewSide>("source");
  return (
    <div className="flex max-w-md flex-col gap-3">
      <button
        type="button"
        className="self-start rounded-md border px-3 py-1 text-sm"
        onClick={() => setSide((s) => (s === "source" ? "fr-FR" : "source"))}
      >
        Show {side === "source" ? "target (fr-FR)" : "source"}
      </button>
      <FormatPreview tree={docxTree} side={side} transition={effect} />
    </div>
  );
}

export const Typewriter: Story = {
  name: "Transition · typewriter",
  render: () => <TransitionDemo effect="typewriter" />,
};

export const Crossfade: Story = {
  name: "Transition · crossfade",
  render: () => <TransitionDemo effect="crossfade" />,
};
