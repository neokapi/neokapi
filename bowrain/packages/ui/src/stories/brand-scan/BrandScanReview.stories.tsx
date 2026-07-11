import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { BrandScanReview } from "../../brand-scan/BrandScanReview";
import { withProviders } from "../decorators";
import { sampleBrandScanDraft } from "../mock-adapter";

const meta: Meta<typeof BrandScanReview> = {
  title: "Brand Scan/BrandScanReview",
  component: BrandScanReview,
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
type Story = StoryObj<typeof BrandScanReview>;

/** A completed draft with confidence, attribution, terms, and the live tester. */
export const CompletedDraft: Story = {
  args: {
    draft: sampleBrandScanDraft,
    onApproved: fn(),
    onRegenerate: fn(),
  },
};

/** A truncated corpus notes that the draft rests on a sample. */
export const TruncatedCorpus: Story = {
  args: {
    draft: { ...sampleBrandScanDraft, truncated: true },
    onApproved: fn(),
    onRegenerate: fn(),
  },
};
