import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { FeatureOverrideDialog } from "./FeatureOverrideDialog";

const meta: Meta<typeof FeatureOverrideDialog> = {
  title: "Ctrl/FeatureOverrideDialog",
  component: FeatureOverrideDialog,
  parameters: { layout: "fullscreen" },
  args: { onOpenChange: fn(), workspaceId: "ws_acme" },
};

export default meta;
type Story = StoryObj<typeof FeatureOverrideDialog>;

export const Open: Story = {
  args: { open: true },
};
