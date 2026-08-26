import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Run } from "@neokapi/contract-types";
import { BlockInspector, FormatPreview } from "@neokapi/ui-primitives/preview";
import type { ContentNode, PreviewSide } from "@neokapi/ui-primitives/preview";
import {
  blockToContentNode,
  blocksToContentTree,
  type BlockEvidence,
} from "../../preview/toContentTree";
import type { BlockInfo, BlockTermMatch, EntityInfo } from "../../types/api";

// The reference wiring for the run-native path: a REST blocks payload plus the
// evidence a review surface loads, projected by `preview/toContentTree` and
// rendered by the shared preview kit — the same components kapi-lab and
// kapi-desktop use. Nothing here re-derives a position: overlay spans come from
// the projection, anchored to the block's runs.

const sourceRuns: Run[] = [
  { text: "Utilize the " },
  { pcOpen: { id: "1", type: "link", data: "<a>", equiv: "a", attrs: { href: "/console" } } },
  { text: "Acme" },
  { pcClose: { id: "1", type: "link", data: "</a>", equiv: "a" } },
  { text: " console to reset your password." },
];

const entities: EntityInfo[] = [
  { key: "entity:0", text: "Acme", type: "entity:organization", start: 12, end: 16, dnt: true },
];

const terms: BlockTermMatch[] = [
  {
    source_term: "console",
    target_terms: ["console"],
    domain: "product",
    status: "preferred",
    start: 21,
    end: 28,
  },
];

const evidence: BlockEvidence = {
  terms,
  findings: [
    {
      category: "voice-vocabulary",
      severity: "major",
      message: 'Forbidden term "Utilize" found',
      suggestion: 'Use "Use" instead',
      original_text: "Utilize",
      position: { kind: "range", start: { run: 0 }, end: { run: 0, offset: 7 } },
      metadata: { replacement: "Use" },
    },
  ],
  issues: [{ type: "spacing", severity: "warning", message: "Trailing double space" }],
  issueLocale: "fr-FR",
};

const blocks: BlockInfo[] = [
  {
    id: "blk-1",
    source: "Utilize the Acme console to reset your password.",
    source_runs: sourceRuns,
    targets: {
      "fr-FR": {
        text: "Utilisez la console Acme pour réinitialiser votre mot de passe.",
        status: "translated",
      },
    },
    targets_runs: {
      "fr-FR": [
        { text: "Utilisez la " },
        { pcOpen: { id: "1", type: "link", data: "<a>", equiv: "a", attrs: { href: "/console" } } },
        { text: "console Acme" },
        { pcClose: { id: "1", type: "link", data: "</a>", equiv: "a" } },
        { text: " pour réinitialiser votre mot de passe." },
      ],
    },
    translatable: true,
    has_spans: true,
    properties: { type: "json:value", path: "$.auth.reset" },
    entities,
  },
  {
    id: "blk-2",
    source: "We will email you a link.",
    source_runs: [{ text: "We will email you a link." }],
    targets: { "fr-FR": { text: "Nous vous enverrons un lien.", status: "reviewed" } },
    translatable: true,
    has_spans: false,
    properties: { type: "json:value", path: "$.auth.sent" },
  },
];

interface ProjectedPreviewProps {
  side: PreviewSide;
  annotations: boolean;
}

function ProjectedPreview({ side, annotations }: ProjectedPreviewProps) {
  const tree = blocksToContentTree(blocks, {
    format: "json",
    name: "auth.json",
    sourceLocale: "en-US",
    evidence: { "blk-1": evidence },
  });
  const node: ContentNode = blockToContentNode(blocks[0], { evidence, sourceLocale: "en-US" });

  return (
    <div className="flex flex-col gap-4 p-4">
      <FormatPreview tree={tree} side={side} annotations={annotations} />
      <BlockInspector node={node} defaultOpen />
    </div>
  );
}

const meta: Meta<typeof ProjectedPreview> = {
  title: "Preview/RunNativeProjection",
  component: ProjectedPreview,
  parameters: { layout: "fullscreen" },
  args: { side: "source", annotations: true },
  argTypes: {
    side: { control: "radio", options: ["source", "fr-FR"] },
  },
};

export default meta;
type Story = StoryObj<typeof ProjectedPreview>;

/** Source side: the voice, term and entity overlays anchored to the source runs. */
export const Source: Story = {};

/** The French target, rendered from its own typed runs. */
export const Target: Story = { args: { side: "fr-FR" } };

/** Annotations off — the same projection without overlay highlighting. */
export const WithoutAnnotations: Story = { args: { annotations: false } };
