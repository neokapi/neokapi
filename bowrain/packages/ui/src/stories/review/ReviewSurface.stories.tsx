import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
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

/** With findings loaded, the flagged blocks are tinted where they sit. */
export const WithFindings: Story = {
  decorators: [
    createProvidersDecorator(sampleBlocks, {
      runFileCheck: async () => [
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

/**
 * A block opened with everything the server resolved behind it: the
 * content-memory wording the bulk pass would otherwise apply unseen, the
 * findings behind its voice score with their suggestions, the last decision
 * and its note, and how the target was produced.
 */
export const InspectorWithContext: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(await canvas.findByTestId("review-block-blk-1"));
    await expect(await canvas.findByTestId("review-inspector")).toBeInTheDocument();
  },
};

/**
 * The same inspector for a block nothing governs, nothing matched and nobody
 * has decided. Every layer names its own emptiness rather than leaving a gap.
 */
export const InspectorWithoutContext: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(await canvas.findByTestId("review-block-blk-3"));
    await expect(await canvas.findByTestId("review-inspector")).toBeInTheDocument();
  },
};
