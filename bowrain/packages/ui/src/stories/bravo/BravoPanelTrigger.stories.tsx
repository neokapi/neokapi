import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { BravoPanelTrigger } from "../../components/bravo/BravoPanelTrigger";

const meta: Meta<typeof BravoPanelTrigger> = {
  title: "Bravo/BravoPanelTrigger",
  component: BravoPanelTrigger,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof BravoPanelTrigger>;

export const Default: Story = {
  args: {
    onClick: fn(),
  },
};

export const Active: Story = {
  args: {
    onClick: fn(),
    active: true,
  },
};

export const WithUnread: Story = {
  args: {
    onClick: fn(),
    hasUnread: true,
  },
};
