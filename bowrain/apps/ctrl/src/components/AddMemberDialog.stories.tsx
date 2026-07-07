import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { AddMemberDialog } from "./AddMemberDialog";

// The dialog reads user search + add-member through the /api/admin client, but
// the search query only fires after 2+ typed characters and the mutation only
// on submit — so the default (open) story renders without any network call.

const meta: Meta<typeof AddMemberDialog> = {
  title: "Ctrl/AddMemberDialog",
  component: AddMemberDialog,
  parameters: { layout: "fullscreen" },
  args: { onOpenChange: fn(), workspaceId: "ws_acme" },
};

export default meta;
type Story = StoryObj<typeof AddMemberDialog>;

export const Open: Story = {
  args: { open: true },
};
