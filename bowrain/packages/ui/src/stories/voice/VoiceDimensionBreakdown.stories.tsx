import type { Meta, StoryObj } from "@storybook/react-vite";
import { VoiceDimensionBreakdown } from "../../voice/VoiceDimensionBreakdown";
import { sampleDimensions, excellentDimensions, poorDimensions } from "./fixtures";

const meta: Meta<typeof VoiceDimensionBreakdown> = {
  title: "Brand/VoiceDimensionBreakdown",
  component: VoiceDimensionBreakdown,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 400, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof VoiceDimensionBreakdown>;

/** Mixed scores across dimensions. */
export const Mixed: Story = {
  args: { dimensions: sampleDimensions },
};

/** All dimensions scoring well. */
export const Excellent: Story = {
  args: { dimensions: excellentDimensions },
};

/** Poor scores across the board. */
export const Poor: Story = {
  args: { dimensions: poorDimensions },
};
