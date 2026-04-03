import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { CreateCollectionDialog } from "../../components/CreateCollectionDialog";
import { docsCollection } from "./fixtures";

const meta: Meta<typeof CreateCollectionDialog> = {
  title: "Workspace/Collections/CreateCollectionDialog",
  component: CreateCollectionDialog,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof CreateCollectionDialog>;

export const Create: Story = {
  args: { open: true, onClose: fn(), onSubmit: fn() },
};

export const Edit: Story = {
  args: { open: true, onClose: fn(), onSubmit: fn(), editCollection: docsCollection },
};
