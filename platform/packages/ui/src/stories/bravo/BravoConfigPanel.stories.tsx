import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { BravoConfigPanel } from "../../components/bravo/BravoConfigPanel";
import { sampleConfig, sampleTools, sampleUsage } from "./fixtures";

const meta: Meta<typeof BravoConfigPanel> = {
  title: "Bravo/BravoConfigPanel",
  component: BravoConfigPanel,
  tags: ["autodocs"],
  decorators: [(Story) => <div style={{ maxWidth: 400, border: "1px solid #ddd", borderRadius: 8 }}><Story /></div>],
};

export default meta;
type Story = StoryObj<typeof BravoConfigPanel>;

export const Default: Story = {
  args: {
    config: sampleConfig,
    tools: sampleTools,
    usage: sampleUsage,
    onSave: fn(),
  },
};

export const Disabled: Story = {
  args: {
    config: { ...sampleConfig, enabled: false },
    tools: sampleTools,
    onSave: fn(),
  },
};

export const NoUsage: Story = {
  args: {
    config: sampleConfig,
    tools: sampleTools,
    onSave: fn(),
  },
};

export const Saving: Story = {
  args: {
    config: sampleConfig,
    tools: sampleTools,
    usage: sampleUsage,
    onSave: fn(),
    saving: true,
  },
};
