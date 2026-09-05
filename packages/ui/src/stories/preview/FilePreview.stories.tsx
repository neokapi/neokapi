import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { fn } from "storybook/test";
import FilePreview from "../../components/preview/FilePreview";
import type { ContentTree } from "../../components/preview/types";

/**
 * A catalog file as the engine returns it: keyed units with a committed French
 * target and a source-anchored term overlay. A `catalog-keyvalue` format, so the
 * viewer reads it as a key table.
 */
const catalog: ContentTree = {
  format: "json",
  root: [
    {
      kind: "block",
      id: "b1",
      name: "app.greeting",
      type: "text",
      translatable: true,
      sourceLocale: "en",
      source: [{ text: "Please utilize the dashboard" }],
      targets: { fr: [{ text: "Veuillez utiliser le tableau de bord" }] },
      targetMeta: { fr: { status: "translated", origin: { kind: "mt", engine: "demo" } } },
      overlays: [
        {
          type: "term",
          side: "source",
          spans: [
            {
              range: { startRun: 0, endRun: 1, startOffset: 19, endOffset: 28 },
              text: "dashboard",
              props: { term: "dashboard", target: "tableau de bord", domain: "ui" },
            },
          ],
        },
      ],
    },
    {
      kind: "block",
      id: "b2",
      name: "app.tagline",
      type: "text",
      translatable: true,
      sourceLocale: "en",
      source: [{ text: "Ship faster" }],
    },
    {
      kind: "block",
      id: "b3",
      name: "release.title",
      type: "text",
      translatable: true,
      sourceLocale: "en",
      source: [{ text: "Publishing a release" }],
      targets: { fr: [{ text: "Publier une version" }] },
    },
  ],
  stats: { layers: 0, groups: 0, blocks: 3, data: 0, media: 0, runs: 3 },
};

/** The same content as a `rich-markup` file, so the viewer lays it out as prose. */
const document: ContentTree = {
  format: "markdown",
  root: [
    {
      kind: "block",
      id: "m1",
      type: "h1",
      translatable: true,
      sourceLocale: "en",
      source: [{ text: "Publishing a release" }],
      targets: { fr: [{ text: "Publier une version" }] },
    },
    {
      kind: "block",
      id: "m2",
      type: "paragraph",
      translatable: true,
      sourceLocale: "en",
      source: [
        {
          text: "A release is cut from the main branch once every check has passed. The steps below assume you have already been granted the publish role.",
        },
      ],
    },
    {
      kind: "block",
      id: "m3",
      type: "h2",
      translatable: true,
      sourceLocale: "en",
      source: [{ text: "Before you begin" }],
    },
    {
      kind: "block",
      id: "m4",
      type: "paragraph",
      translatable: true,
      sourceLocale: "en",
      source: [{ text: "Confirm the changelog names every user-facing change." }],
    },
  ],
  stats: { layers: 0, groups: 0, blocks: 4, data: 0, media: 0, runs: 4 },
};

const meta: Meta<typeof FilePreview> = {
  title: "Preview/FilePreview",
  component: FilePreview,
  tags: ["autodocs"],
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "The preview sheet every surface reads one file in: the file and its format in the header, the readings on a strip beneath it, the document in a scroll container, and the explicit handoffs in a footer. kapi desktop drives it from the native engine's content tree; the bowrain platform renders its own body inside it and adds its story readings and editor actions.",
      },
    },
  },
  args: {
    open: true,
    onClose: fn(),
    filename: "locales/en.json",
    format: "json",
    description: "Structure, vocabulary and check annotations, and source ↔ target.",
    tree: catalog,
  },
};

export default meta;
type Story = StoryObj<typeof FilePreview>;

/** A catalog file: the viewer reads it as a key table with its French target. */
export const KeyedTable: Story = {};
export const KeyedTableDark: Story = { globals: { theme: "dark" } };

/** A marked-up file: the same viewer lays the content out as a document. */
export const DocumentPreview: Story = {
  args: { filename: "docs/releasing.md", format: "markdown", tree: document },
};
export const DocumentPreviewDark: Story = {
  args: DocumentPreview.args,
  globals: { theme: "dark" },
};

/** The host is still reading the file. */
export const Loading: Story = {
  args: { loading: true, tree: null },
};
export const LoadingDark: Story = { args: Loading.args, globals: { theme: "dark" } };

/** The host could not read it, and says why. */
export const ErrorState: Story = {
  name: "Error",
  args: { error: "No reader is registered for this file type.", tree: null },
};
export const ErrorStateDark: Story = {
  name: "Error (dark)",
  args: ErrorState.args,
  globals: { theme: "dark" },
};

/** Nothing failed and nothing is loading; there is simply nothing to read. */
export const Empty: Story = {
  args: {
    tree: null,
    empty: (
      <p className="py-8 text-sm text-muted-foreground">
        This file carries no translatable content.
      </p>
    ),
  },
};
export const EmptyDark: Story = { args: Empty.args, globals: { theme: "dark" } };

/** The handoffs a surface offers once the reader has seen the file. */
export const WithActions: Story = {
  args: {
    subtitle: "ui/src/App.kbf.json",
    subtitleTestId: "file-preview-item-name",
    actions: (
      <>
        <button
          type="button"
          className="h-8 rounded-md bg-primary px-3 text-xs font-medium text-primary-foreground"
        >
          Open in Translate
        </button>
        <button type="button" className="h-8 rounded-md border px-3 text-xs font-medium">
          Review
        </button>
      </>
    ),
  },
};
export const WithActionsDark: Story = { args: WithActions.args, globals: { theme: "dark" } };

/**
 * A reader arriving from a queue to look at one unit: the block is marked and
 * scrolled to, and the states of the file's other units are drawn alongside it.
 */
export const FocusedUnit: Story = {
  args: {
    focusKey: "app.greeting",
    unitStates: { "app.greeting": "needs work", "release.title": "approved" },
    backLabel: "Back to review",
  },
};
export const FocusedUnitDark: Story = { args: FocusedUnit.args, globals: { theme: "dark" } };

/**
 * Every switch the strip can carry: which language side, which reading, and the
 * host's own locale control. The bowrain platform drives it this way.
 */
function ReadingStrip() {
  const [side, setSide] = useState<"source" | "target">("source");
  const [view, setView] = useState("document");
  return (
    <FilePreview
      open
      onClose={fn()}
      filename="docs/releasing.md"
      format="markdown"
      description="Read the document, then open it in an editor."
      sides={{ value: side, onChange: setSide, targetLabel: "French" }}
      view={view}
      onViewChange={setView}
      viewsLabel="Reading"
      views={[
        { value: "document", label: "Document", content: <p className="py-4">The document.</p> },
        {
          value: "context",
          label: "In context",
          content: <p className="py-4">The component that ships it.</p>,
        },
      ]}
      toolbar={
        <span className="rounded-md border px-2 py-1 text-xs text-muted-foreground">fr-FR</span>
      }
      scrollBody={false}
    />
  );
}

export const Readings: Story = { render: () => <ReadingStrip /> };
export const ReadingsDark: Story = {
  render: () => <ReadingStrip />,
  globals: { theme: "dark" },
};

/**
 * A reader arriving from a list of check findings: the finding they came for is
 * marked and underlined on its block, the file's other findings are drawn
 * dimmer in their own tones, and one block carries two findings at once. The
 * header names the finding rather than a review state, and the way back is to
 * the list of findings.
 */
export const FindingHighlights: Story = {
  args: {
    highlights: {
      b1: [
        {
          side: "source",
          anchor: { kind: "range", start: { run: 0, offset: 7 }, end: { run: 0, offset: 14 } },
          tone: "destructive",
          label: 'Forbidden term "utilize" found',
          emphasis: "focus",
        },
        {
          side: "source",
          anchor: { kind: "range", start: { run: 0, offset: 19 }, end: { run: 1 } },
          tone: "warning",
          label: 'Prefer "overview" to "dashboard" in product copy',
          emphasis: "dim",
        },
      ],
      b2: [
        {
          side: "source",
          anchor: { kind: "block" },
          tone: "muted",
          label: "Tone reads more formal than the brand's register",
          emphasis: "dim",
        },
      ],
    },
    focusKey: "app.greeting",
    focusNote: (
      <>
        <span className="rounded border border-destructive/40 bg-destructive/10 px-1.5 text-[10px] font-medium text-destructive">
          Major
        </span>
        <span className="text-muted-foreground">Forbidden term "utilize" found</span>
      </>
    ),
    backLabel: "Back to checks",
  },
};
export const FindingHighlightsDark: Story = {
  args: FindingHighlights.args,
  globals: { theme: "dark" },
};

/**
 * A finding about the translation: the document opens on the target side with
 * the block marked whole, because the checker's position is anchored to the
 * source runs, where the words it means are underlined once the reader toggles
 * to the source.
 */
export const FindingHighlightsTargetSide: Story = {
  args: {
    viewer: { defaultSide: "fr" },
    highlights: {
      b3: [
        {
          side: "fr",
          anchor: { kind: "block" },
          tone: "destructive",
          label: 'Do-not-translate term "release" is missing from the fr target',
          emphasis: "focus",
        },
        {
          side: "source",
          anchor: { kind: "range", start: { run: 0, offset: 13 }, end: { run: 1 } },
          tone: "destructive",
          label: 'Do-not-translate term "release" is missing from the fr target',
          emphasis: "focus",
        },
      ],
    },
    focusKey: "release.title",
    focusNote: (
      <>
        <span className="rounded border border-destructive/40 bg-destructive/10 px-1.5 text-[10px] font-medium text-destructive">
          Critical
        </span>
        <span className="text-muted-foreground">
          Do-not-translate term "release" is missing from the fr target
        </span>
      </>
    ),
    backLabel: "Back to checks",
  },
};
export const FindingHighlightsTargetSideDark: Story = {
  args: FindingHighlightsTargetSide.args,
  globals: { theme: "dark" },
};

/** The same marks on a document laid out as prose rather than as a key table. */
export const FindingHighlightsDocument: Story = {
  args: {
    filename: "docs/releasing.md",
    format: "markdown",
    tree: document,
    highlights: {
      m2: [
        {
          side: "source",
          anchor: { kind: "range", start: { run: 0, offset: 61 }, end: { run: 0, offset: 67 } },
          tone: "destructive",
          label: 'Say "sign in" rather than "assume": name the step the reader takes',
          emphasis: "focus",
        },
      ],
      m4: [
        {
          side: "source",
          anchor: { kind: "block" },
          tone: "warning",
          label: "A checklist item reads as an instruction; state it as a check",
          emphasis: "dim",
        },
      ],
    },
    backLabel: "Back to checks",
    focusNote: (
      <span className="text-muted-foreground">
        Say "sign in" rather than "assume": name the step the reader takes
      </span>
    ),
  },
};
export const FindingHighlightsDocumentDark: Story = {
  args: FindingHighlightsDocument.args,
  globals: { theme: "dark" },
};
