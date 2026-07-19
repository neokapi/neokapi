import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { SourceProposalsDialog } from "../../components/review/SourceProposalsDialog";
import { createProvidersDecorator } from "../decorators";
import type { SourceProposal } from "../../types/api";

const now = new Date(0).toISOString();
const sampleProposals: SourceProposal[] = [
  {
    id: "sp-1",
    workspace_id: "ws",
    project_id: "proj-1",
    stream: "main",
    item_name: "ui.json",
    block_id: "b1",
    kind: "text-fix",
    original_source: "Colour picker",
    proposed_source: "Color picker",
    rationale: "House style is US English.",
    found_in_locale: "fr-FR",
    finder_user: "reviewer-1",
    status: "open",
    created_at: now,
    updated_at: now,
  },
  {
    id: "sp-2",
    workspace_id: "ws",
    project_id: "proj-1",
    item_name: "marketing.json",
    block_id: "b7",
    kind: "mark-dnt",
    original_source: "kubectl",
    proposed_source: "kubectl",
    found_in_locale: "de-DE",
    status: "open",
    created_at: now,
    updated_at: now,
  },
];

const meta: Meta<typeof SourceProposalsDialog> = {
  title: "Review/SourceProposalsDialog",
  component: SourceProposalsDialog,
  args: {
    open: true,
    onOpenChange: fn(),
    projectId: "proj-1",
    localeName: (c: string) => (c === "fr-FR" ? "French" : c === "de-DE" ? "German" : c),
    onDone: fn(),
  },
};
export default meta;
type Story = StoryObj<typeof SourceProposalsDialog>;

/** A source owner reviewing two open proposals. */
export const WithProposals: Story = {
  decorators: [
    createProvidersDecorator(undefined, {
      listSourceProposals: async () => sampleProposals,
    }),
  ],
};

/** Nothing to review. */
export const Empty: Story = {
  decorators: [
    createProvidersDecorator(undefined, {
      listSourceProposals: async () => [],
    }),
  ],
};
