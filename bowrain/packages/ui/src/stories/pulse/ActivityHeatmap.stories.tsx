import type { Meta, StoryObj } from "@storybook/react";
import { ActivityHeatmap } from "../../components/pulse";
import { mockHeatmapDays } from "./pulse-fixtures";

const meta: Meta<typeof ActivityHeatmap> = {
  title: "Pulse/ActivityHeatmap",
  component: ActivityHeatmap,
};

export default meta;
type Story = StoryObj<typeof ActivityHeatmap>;

export const Default: Story = {
  args: {
    days: mockHeatmapDays,
  },
};

export const Empty: Story = {
  args: {
    days: [],
  },
};

export const Sparse: Story = {
  args: {
    days: [
      { date: "2026-03-01", count: 1 },
      { date: "2026-03-10", count: 5 },
      { date: "2026-03-20", count: 12 },
    ],
  },
};
