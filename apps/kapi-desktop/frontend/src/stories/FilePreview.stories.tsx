import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { FilePreview } from "../components/FilePreview";
import type { ContentTree } from "@neokapi/kapi-lab";

// A small ContentTree shaped exactly like editor.BuildContentTree's JSON, with a
// source run sequence, a committed fr target, and source-anchored term + brand +
// QA overlays — the shape InspectFileAnnotated returns. Supplying it as the
// `tree` prop drives the real DocumentViewer without a Wails backend.
const sampleTree: ContentTree = {
  format: "json",
  root: [
    {
      kind: "block",
      id: "greeting",
      name: "greeting",
      type: "text",
      translatable: true,
      sourceLocale: "en",
      source: [{ text: "Please utilize the dashboard" }],
      targets: {
        fr: [{ text: "Veuillez utiliser le tableau de bord" }],
      },
      targetMeta: {
        fr: { status: "translated", origin: { kind: "mt", engine: "demo" } },
      },
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
        {
          type: "qa",
          side: "source",
          spans: [
            {
              range: { startRun: 0, endRun: 1, startOffset: 7, endOffset: 14 },
              text: "utilize",
              props: {
                category: "brand-vocabulary",
                severity: "major",
                term: "utilize",
                kind: "forbidden",
                replacement: "use",
                message: 'Forbidden term "utilize" found',
              },
            },
          ],
        },
      ],
    },
    {
      kind: "block",
      id: "tagline",
      name: "tagline",
      type: "text",
      translatable: true,
      sourceLocale: "en",
      source: [{ text: "Ship faster" }],
    },
  ],
  stats: { layers: 0, groups: 0, blocks: 2, data: 0, media: 0, runs: 2 },
};

const meta: Meta<typeof FilePreview> = {
  title: "Pages/FilePreview",
  component: FilePreview,
  parameters: { layout: "fullscreen" },
  args: {
    tabID: "tab-1",
    filePath: "/Users/dev/acme-app/locales/en.json",
    filename: "locales/en.json",
    onClose: fn(),
    tree: sampleTree,
  },
};

export default meta;
type Story = StoryObj<typeof FilePreview>;

// Default: the preview sheet open on a JSON file, showing the source, the fr
// target (via the side toggle) and the term / brand-vocabulary annotations.
export const Default: Story = {};

// A source-only file (no committed targets): the side toggle offers only Source.
export const SourceOnly: Story = {
  args: {
    filename: "locales/en.json",
    tree: {
      ...sampleTree,
      root: [sampleTree.root[1]],
      stats: { ...sampleTree.stats, blocks: 1, runs: 1 },
    },
  },
};
