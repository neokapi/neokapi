import type { Meta, StoryObj } from "@storybook/react-vite";
import { VoiceDashboard } from "../../voice/VoiceDashboard";
import {
  sampleScore,
  excellentScore,
  poorScore,
  sampleTrends,
  sampleStoredScores,
} from "./fixtures";

const meta: Meta<typeof VoiceDashboard> = {
  title: "Brand/VoiceDashboard",
  component: VoiceDashboard,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 960, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof VoiceDashboard>;

/** No compliance data yet. */
export const Empty: Story = {
  args: {
    score: null,
    trends: [],
    recentScores: [],
  },
};

/** Typical mixed-score dashboard. */
export const WithData: Story = {
  args: {
    score: sampleScore,
    trends: sampleTrends,
    recentScores: sampleStoredScores,
  },
};

/** Excellent compliance across the board. */
export const Excellent: Story = {
  args: {
    score: excellentScore,
    trends: sampleTrends.map((t) => ({ ...t, avg_score: t.avg_score + 20 })),
    recentScores: sampleStoredScores.slice(0, 2),
  },
};

/** Poor compliance with many findings. */
export const Poor: Story = {
  args: {
    score: poorScore,
    trends: sampleTrends.map((t) => ({ ...t, avg_score: Math.max(10, t.avg_score - 30) })),
    recentScores: sampleStoredScores,
  },
};

/** Data but no trend history yet. */
export const NoTrends: Story = {
  args: {
    score: sampleScore,
    trends: [],
    recentScores: sampleStoredScores,
  },
};
