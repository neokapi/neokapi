import type { Meta, StoryObj } from "@storybook/react";
import { PulseHeader } from "../../components/pulse";

const meta: Meta<typeof PulseHeader> = {
  title: "Pulse/PulseHeader",
  component: PulseHeader,
  parameters: { layout: "padded" },
};

export default meta;
type Story = StoryObj<typeof PulseHeader>;

export const Default: Story = { args: { workspaceName: "My Workspace" } };
export const WithLogo: Story = {
  args: { workspaceName: "My Workspace", logoUrl: "https://via.placeholder.com/32" },
};
export const LongName: Story = {
  args: { workspaceName: "Acme Corporation International Localization Team" },
};
