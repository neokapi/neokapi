import type { Meta, StoryObj } from "@storybook/react";
import { PulseOverview } from "../../components/pulse";
import { mockStats, mockProjects, mockLanguages, mockHeatmapDays } from "./pulse-fixtures";

const meta: Meta<typeof PulseOverview> = {
  title: "Pulse/PulseOverview",
  component: PulseOverview,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof PulseOverview>;

export const Default: Story = {
  args: {
    stats: mockStats,
    projects: mockProjects,
    languages: mockLanguages,
    heatmap: mockHeatmapDays,
  },
};

export const Empty: Story = {
  args: {
    stats: {
      total_projects: 0,
      total_languages: 0,
      total_contributors: 0,
      total_words: 0,
      translated_words: 0,
      overall_percent: 0,
    },
    projects: [],
    languages: [],
  },
};
