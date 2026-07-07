import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { GrantCreditsDialog } from "./GrantCreditsDialog";

const meta: Meta<typeof GrantCreditsDialog> = {
  title: "Ctrl/GrantCreditsDialog",
  component: GrantCreditsDialog,
  parameters: { layout: "fullscreen" },
  args: { onOpenChange: fn(), workspaceId: "ws_acme" },
};

export default meta;
type Story = StoryObj<typeof GrantCreditsDialog>;

export const Open: Story = {
  args: { open: true },
};
