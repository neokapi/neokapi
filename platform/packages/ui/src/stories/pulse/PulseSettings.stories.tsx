import type { Meta, StoryObj } from "@storybook/react";
import { PulseSettings } from "../../components/pulse";

const meta: Meta<typeof PulseSettings> = {
  title: "Pulse/PulseSettings",
  component: PulseSettings,
  parameters: { layout: "padded" },
  args: {
    workspaceSlug: "acme-corp",
    onVisibilityChange: async () => {},
  },
};

export default meta;
type Story = StoryObj<typeof PulseSettings>;

export const Private: Story = { args: { visibility: "private" } };
export const Unlisted: Story = { args: { visibility: "unlisted" } };
export const Public: Story = { args: { visibility: "public" } };

export const CustomPulseUrl: Story = {
  args: {
    visibility: "public",
    pulseBaseUrl: "https://pulse.example.com",
  },
};
