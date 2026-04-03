import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { BravoUsageDashboard } from "../../components/bravo/BravoUsageDashboard";
import { sampleUsage } from "./fixtures";

const meta: Meta<typeof BravoUsageDashboard> = {
  title: "Bravo/BravoUsageDashboard",
  component: BravoUsageDashboard,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 600, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof BravoUsageDashboard>;

export const Default: Story = {
  args: {
    usage: sampleUsage,
  },
};

export const WithDateRange: Story = {
  args: {
    usage: sampleUsage,
    onDateRangeChange: fn(),
  },
};

export const LowUsage: Story = {
  args: {
    usage: {
      workspace_id: "ws-1",
      total_input_tokens: 1200,
      total_output_tokens: 400,
      total_container_sec: 45,
      message_count: 3,
    },
  },
};

export const HighUsage: Story = {
  args: {
    usage: {
      workspace_id: "ws-1",
      total_input_tokens: 2450000,
      total_output_tokens: 890000,
      total_container_sec: 48200,
      message_count: 1240,
    },
    onDateRangeChange: fn(),
  },
};
