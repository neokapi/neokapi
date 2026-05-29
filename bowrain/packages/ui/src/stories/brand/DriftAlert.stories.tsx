import type { Meta, StoryObj } from "@storybook/react-vite";
import { DriftAlert } from "../../brand/DriftAlert";

const meta: Meta<typeof DriftAlert> = {
  title: "Brand/DriftAlert",
  component: DriftAlert,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 560, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof DriftAlert>;

/** Compliance has dropped from its baseline — a warning banner. */
export const Drifting: Story = {
  args: {
    drift: {
      drifted: true,
      recent_avg: 71.2,
      baseline_avg: 95,
      drop: 23.8,
      recent_days: 7,
      recent_count: 30,
      reason: "recent average dropped from baseline",
    },
  },
};

/** Stable and quiet — by default renders nothing; here with showStable. */
export const Stable: Story = {
  args: {
    showStable: true,
    drift: {
      drifted: false,
      recent_avg: 92,
      baseline_avg: 91,
      drop: -1,
      recent_days: 7,
      recent_count: 30,
    },
  },
};
