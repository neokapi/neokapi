import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ReviewSurface } from "../../components/ReviewSurface";
import { createProvidersDecorator } from "../decorators";
import { sampleBlocks, sampleProject } from "../fixtures";

// The review surface as the reader meets it: the item rendered as a document,
// each block carrying its review status in the margin, and the inspector
// arriving beside the text when a block is opened.

const meta: Meta<typeof ReviewSurface> = {
  title: "Review/ReviewSurface",
  component: ReviewSurface,
  parameters: { layout: "fullscreen" },
  decorators: [
    createProvidersDecorator(sampleBlocks),
    (Story) => (
      <div className="flex h-[42rem] flex-col p-4">
        <Story />
      </div>
    ),
  ],
  args: {
    project: sampleProject,
    fileName: "messages.json",
    onBack: fn(),
  },
};

export default meta;
type Story = StoryObj<typeof ReviewSurface>;

/** The document, read on the target locale. */
export const Default: Story = {};

/** With QA findings loaded, the flagged blocks are tinted where they sit. */
export const WithFindings: Story = {
  decorators: [
    createProvidersDecorator(sampleBlocks, {
      runFileQACheck: async () => [
        {
          blockId: "blk-1",
          issues: [
            { type: "spacing", severity: "warning", message: "Trailing double space" },
            { type: "placeholder", severity: "error", message: "Missing {count} in the target" },
          ],
        },
      ],
    }),
  ],
};
