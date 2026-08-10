import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { FilePreview } from "../../components/FilePreview";
import { createProvidersDecorator } from "../decorators";
import type { BlockInfo } from "../../types/api";

/**
 * A markdown document as the blocks payload carries it: the reader assigned each
 * block a type, so the preview kit lays headings, body copy and a fenced code
 * block out as a document rather than a list of strings.
 */
const markdownBlocks: BlockInfo[] = [
  {
    id: "md-1",
    source: "Publishing a release",
    source_coded: "Publishing a release",
    source_spans: [],
    targets: { "fr-FR": "Publier une version", "de-DE": "Eine Version veröffentlichen" },
    targets_coded: { "fr-FR": "Publier une version", "de-DE": "Eine Version veröffentlichen" },
    translatable: true,
    has_spans: false,
    properties: { type: "h1" },
  },
  {
    id: "md-2",
    source:
      "A release is cut from the main branch once every check has passed. The steps below assume you have already been granted the publish role.",
    source_coded:
      "A release is cut from the main branch once every check has passed. The steps below assume you have already been granted the publish role.",
    source_spans: [],
    targets: {
      "fr-FR":
        "Une version est créée à partir de la branche principale une fois que tous les contrôles sont passés. Les étapes ci-dessous supposent que le rôle de publication vous a déjà été accordé.",
    },
    targets_coded: {
      "fr-FR":
        "Une version est créée à partir de la branche principale une fois que tous les contrôles sont passés. Les étapes ci-dessous supposent que le rôle de publication vous a déjà été accordé.",
    },
    translatable: true,
    has_spans: false,
    properties: { type: "paragraph" },
  },
  {
    id: "md-3",
    source: "Before you begin",
    source_coded: "Before you begin",
    source_spans: [],
    targets: { "fr-FR": "Avant de commencer" },
    targets_coded: { "fr-FR": "Avant de commencer" },
    translatable: true,
    has_spans: false,
    properties: { type: "h2" },
  },
  {
    id: "md-4",
    source: "Confirm the changelog names every user-facing change.",
    source_coded: "Confirm the changelog names every user-facing change.",
    source_spans: [],
    targets: { "fr-FR": "Vérifiez que le journal des modifications nomme chaque changement." },
    targets_coded: {
      "fr-FR": "Vérifiez que le journal des modifications nomme chaque changement.",
    },
    translatable: true,
    has_spans: false,
    properties: { type: "bullet" },
  },
  {
    id: "md-5",
    source: "Confirm the target languages have caught up, or note the ones that have not.",
    source_coded: "Confirm the target languages have caught up, or note the ones that have not.",
    source_spans: [],
    targets: {},
    targets_coded: {},
    translatable: true,
    has_spans: false,
    properties: { type: "bullet" },
  },
  {
    id: "md-6",
    source: "make release VERSION=1.2.0",
    source_coded: "make release VERSION=1.2.0",
    source_spans: [],
    targets: {},
    targets_coded: {},
    translatable: false,
    has_spans: false,
    properties: { type: "code", "code.language": "bash" },
  },
  {
    id: "md-7",
    source:
      "The tag is pushed by the release job, not from a laptop. If the job fails after tagging, delete the tag before retrying.",
    source_coded:
      "The tag is pushed by the release job, not from a laptop. If the job fails after tagging, delete the tag before retrying.",
    source_spans: [],
    targets: {
      "fr-FR":
        "Le tag est poussé par le job de publication, pas depuis un poste de travail. Si le job échoue après le tag, supprimez le tag avant de réessayer.",
    },
    targets_coded: {
      "fr-FR":
        "Le tag est poussé par le job de publication, pas depuis un poste de travail. Si le job échoue après le tag, supprimez le tag avant de réessayer.",
    },
    translatable: true,
    has_spans: false,
    properties: { type: "paragraph" },
  },
];

/**
 * The marker `core/editor` puts on a fallback preview: no reader supplied HTML
 * for this format, so the kit renders the document from the content model. The
 * markdown reader is one of those formats, which is what these stories exercise.
 */
const genericPreview = '<div data-kat-preview="generic"></div>';

const meta: Meta<typeof FilePreview> = {
  title: "Pages/Workspace/FilePreview",
  component: FilePreview,
  decorators: [
    createProvidersDecorator(markdownBlocks, {
      renderDocumentPreview: async () => genericPreview,
    }),
  ],
  args: {
    projectId: "proj-1",
    itemName: "docs/releasing.md",
    format: "markdown",
    targetLocales: ["fr-FR", "de-DE"],
    onClose: fn(),
    onOpenTranslate: fn(),
    onOpenReview: fn(),
    onOpenPreProcess: fn(),
  },
};

export default meta;
type Story = StoryObj<typeof FilePreview>;

/** The source reading — the document as it was authored. */
export const Markdown: Story = {};

/** One target locale, where the project declares only that one. */
export const SingleTargetLocale: Story = {
  args: { targetLocales: ["fr-FR"] },
};

/**
 * A format whose reader renders its own HTML. The server's preview carries no
 * generic marker, so the document stays in an iframe rather than being re-laid
 * out from the content model.
 */
export const ReaderSuppliedHTML: Story = {
  decorators: [
    createProvidersDecorator(markdownBlocks, {
      renderDocumentPreview: async () =>
        "<html><body style='font-family:system-ui;padding:24px'><h1>Publishing a release</h1><p>Rendered by the format's own reader.</p></body></html>",
    }),
  ],
  args: { itemName: "docs/releasing.html", format: "html" },
};

/** No target languages yet: the reading toggle has nothing to offer, so it is absent. */
export const SourceOnly: Story = {
  args: { targetLocales: [] },
};
