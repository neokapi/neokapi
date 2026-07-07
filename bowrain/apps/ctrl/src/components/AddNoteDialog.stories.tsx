import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { AddNoteDialog } from "./AddNoteDialog";

const meta: Meta<typeof AddNoteDialog> = {
  title: "Ctrl/AddNoteDialog",
  component: AddNoteDialog,
  parameters: { layout: "fullscreen" },
  args: { onOpenChange: fn(), workspaceId: "ws_acme" },
};

export default meta;
type Story = StoryObj<typeof AddNoteDialog>;

export const Open: Story = {
  args: { open: true },
};
