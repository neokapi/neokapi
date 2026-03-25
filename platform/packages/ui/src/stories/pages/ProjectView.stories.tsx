import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
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
    onEditProject: fn(),
    onArchiveProject: fn(),
    onManageMembers: fn(),
    onTogglePulseVisibility: fn(),
    onCreateCollection: fn(),
    onEditCollection: fn(),
    onDeleteCollection: fn(),
    onUploadToCollection: fn(),
    onCreateStream: fn(),
    onMergeStream: fn(),
    onDiffStream: fn(),
    onDeleteStream: fn(),
  },
};

export const PublicOnPulse: Story = {
  args: {
    ...Default.args,
    project: { ...sampleProject, dashboard_visibility: "public" },
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
    onCreateCollection: fn(),
    onEditCollection: fn(),
    onDeleteCollection: fn(),
    onCreateStream: fn(),
  },
};

export const EmptyProject: Story = {
  args: {
    project: {
      ...sampleProject,
      items: [],
      collections: sampleProject.collections?.map((c) => ({ ...c, item_count: 0 })),
    },
    onBack: fn(),
    onOpenFile: fn(),
    onUploadFiles: fn(),
    onRemoveFile: fn(),
    onCreateCollection: fn(),
    onCreateStream: fn(),
  },
};

export const SingleCollection: Story = {
  args: {
    project: {
      ...sampleProject,
      collections: sampleProject.collections?.slice(0, 1),
    },
    onBack: fn(),
    onOpenFile: fn(),
    onUploadFiles: fn(),
    onRemoveFile: fn(),
  },
};
