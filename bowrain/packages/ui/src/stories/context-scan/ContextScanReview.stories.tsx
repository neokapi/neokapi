import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ContextScanReview } from "../../context-scan/ContextScanReview";
import { withProviders } from "../decorators";
import { sampleContextScanDraft } from "../mock-adapter";

const meta: Meta<typeof ContextScanReview> = {
  title: "Brand Scan/ContextScanReview",
  component: ContextScanReview,
  tags: ["autodocs"],
  decorators: [
    withProviders,
    (Story) => (
      <div style={{ maxWidth: 1100, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ContextScanReview>;

/** A completed draft with confidence, attribution, terms, and the live tester. */
export const CompletedDraft: Story = {
  args: {
    draft: sampleContextScanDraft,
    onApproved: fn(),
    onRegenerate: fn(),
  },
};

/** A truncated corpus notes that the draft rests on a sample. */
export const TruncatedCorpus: Story = {
  args: {
    draft: { ...sampleContextScanDraft, truncated: true },
    onApproved: fn(),
    onRegenerate: fn(),
  },
};
