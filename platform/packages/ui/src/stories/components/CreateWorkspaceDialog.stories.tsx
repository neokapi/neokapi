import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { CreateWorkspaceDialog } from "../../components/CreateWorkspaceDialog";
import { withProviders } from "../decorators";

const meta: Meta<typeof CreateWorkspaceDialog> = {
  title: "Components/CreateWorkspaceDialog",
  component: CreateWorkspaceDialog,
  tags: ["autodocs"],
  decorators: [withProviders],
};

export default meta;
type Story = StoryObj<typeof CreateWorkspaceDialog>;

export const Default: Story = {
  args: { open: true, onOpenChange: fn(), onCreate: fn() },
};
