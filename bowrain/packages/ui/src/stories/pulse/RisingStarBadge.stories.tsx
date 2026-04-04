import type { Meta, StoryObj } from "@storybook/react";
import { RisingStarBadge } from "../../components/pulse";

const meta: Meta<typeof RisingStarBadge> = {
  title: "Pulse/RisingStarBadge",
  component: RisingStarBadge,
  tags: ["autodocs"],
  parameters: { layout: "centered" },
};

export default meta;
type Story = StoryObj<typeof RisingStarBadge>;

export const Positive: Story = { args: { growth: 12.5 } };
export const Negative: Story = { args: { growth: -5.3 } };
export const Zero: Story = { args: { growth: 0 } };
export const Large: Story = { args: { growth: 150 } };
