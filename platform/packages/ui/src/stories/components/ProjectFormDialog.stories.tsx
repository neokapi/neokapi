import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ProjectFormDialog } from "../../components/ProjectFormDialog";
import { sampleProject } from "../fixtures";
import { withProviders } from "../decorators";

const meta: Meta<typeof ProjectFormDialog> = {
  title: "Workspace/Projects/ProjectFormDialog",
  component: ProjectFormDialog,
  tags: ["autodocs"],
  decorators: [withProviders],
};

export default meta;
type Story = StoryObj<typeof ProjectFormDialog>;

export const Create: Story = {
  args: {
    open: true,
    onOpenChange: fn(),
    onSubmit: fn(),
    workspaceLanguages: ["en", "fr", "de", "ja", "es", "pt"],
  },
};

export const Edit: Story = {
  args: {
    open: true,
    onOpenChange: fn(),
    onSubmit: fn(),
    editProject: sampleProject,
    workspaceLanguages: ["en", "fr", "de", "ja", "es", "pt"],
  },
};

export const NoWorkspaceLanguages: Story = {
  args: {
    open: true,
    onOpenChange: fn(),
    onSubmit: fn(),
  },
};
