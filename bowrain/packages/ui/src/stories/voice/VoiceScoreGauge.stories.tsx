import type { Meta, StoryObj } from "@storybook/react-vite";
import { VoiceScoreGauge } from "../../voice/VoiceScoreGauge";

const meta: Meta<typeof VoiceScoreGauge> = {
  title: "Brand/VoiceScoreGauge",
  component: VoiceScoreGauge,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ display: "flex", gap: 32, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof VoiceScoreGauge>;

/** High score (green). */
export const Excellent: Story = {
  args: { score: 93, label: "Overall" },
};

/** Medium score (yellow). */
export const Good: Story = {
  args: { score: 68, label: "Tone" },
};

/** Low-medium score (orange). */
export const Fair: Story = {
  args: { score: 45, label: "Vocabulary" },
};

/** Poor score (red). */
export const Poor: Story = {
  args: { score: 22, label: "Style" },
};

/** Custom size. */
export const LargeGauge: Story = {
  args: { score: 85, size: 200, label: "Overall Score" },
};

/** Zero score. */
export const Zero: Story = {
  args: { score: 0, label: "No data" },
};

/** Perfect score. */
export const Perfect: Story = {
  args: { score: 100, label: "Perfect" },
};
