import type { Meta, StoryObj } from "@storybook/react-vite";
import { MetricsCards } from "./MetricsCards";
import { sampleMetrics } from "../ctrl-fixtures";

const meta: Meta<typeof MetricsCards> = {
  title: "Ctrl/MetricsCards",
  component: MetricsCards,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <div className="w-full max-w-4xl">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof MetricsCards>;

export const Default: Story = {
  args: { metrics: sampleMetrics, loading: false },
};

export const Loading: Story = {
  args: { metrics: null, loading: true },
};
