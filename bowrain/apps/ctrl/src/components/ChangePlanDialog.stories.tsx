import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ChangePlanDialog } from "./ChangePlanDialog";

const meta: Meta<typeof ChangePlanDialog> = {
  title: "Ctrl/ChangePlanDialog",
  component: ChangePlanDialog,
  parameters: { layout: "fullscreen" },
  args: { onOpenChange: fn(), workspaceId: "ws_acme", currentPlan: "pro" },
};

export default meta;
type Story = StoryObj<typeof ChangePlanDialog>;

export const Open: Story = {
  args: { open: true },
};

export const EnterprisePlan: Story = {
  args: { open: true, currentPlan: "enterprise" },
};
