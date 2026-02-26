import type { Meta, StoryObj } from "@storybook/react";
import { fn } from "@storybook/test";
import { ProjectView } from "../../components/ProjectView";
import { withProviders } from "../decorators";
import { sampleProject } from "../fixtures";

const meta: Meta<typeof ProjectView> = {
  title: "Pages/ProjectView",
  component: ProjectView,
  tags: ["autodocs"],
  decorators: [
    withProviders,
    (Story) => (
      <div style={{ maxWidth: 960, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ProjectView>;

export const Default: Story = {
  args: {
    project: sampleProject,
    onBack: fn(),
    onOpenFile: fn(),
    onUploadFiles: fn(),
    onRemoveFile: fn(),
    onOpenTM: fn(),
    onOpenTerms: fn(),
  },
};

export const WithServerMode: Story = {
  args: {
    project: sampleProject,
    onBack: fn(),
    onOpenFile: fn(),
    onUploadFiles: fn(),
    onRemoveFile: fn(),
    onOpenTM: fn(),
    onOpenTerms: fn(),
    serverMode: { serverURL: "https://bowrain.example.com", workspaceSlug: "demo" },
  },
};

export const EmptyProject: Story = {
  args: {
    project: {
      ...sampleProject,
      items: [],
    },
    onBack: fn(),
    onOpenFile: fn(),
    onUploadFiles: fn(),
    onRemoveFile: fn(),
  },
};
