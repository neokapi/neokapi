import type { Meta, StoryObj } from "@storybook/react";
import { CompletionRing } from "../../components/pulse";

const meta: Meta<typeof CompletionRing> = {
  title: "Pulse/CompletionRing",
  component: CompletionRing,
  tags: ["autodocs"],
  parameters: { layout: "centered" },
};

export default meta;
type Story = StoryObj<typeof CompletionRing>;

export const Empty: Story = { args: { percentage: 0 } };
export const Quarter: Story = { args: { percentage: 25 } };
export const Half: Story = { args: { percentage: 50 } };
export const AlmostComplete: Story = { args: { percentage: 95 } };
export const Complete: Story = { args: { percentage: 100 } };
export const Large: Story = { args: { percentage: 72, size: 96, strokeWidth: 8 } };
