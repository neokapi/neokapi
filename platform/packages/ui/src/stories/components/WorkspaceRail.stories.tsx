import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { WorkspaceRail } from "../../components/WorkspaceRail";
import { sampleWorkspace, personalWorkspace, sampleUser } from "./fixtures";

const meta: Meta<typeof WorkspaceRail> = {
  title: "Layout/WorkspaceRail",
  component: WorkspaceRail,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ height: 400 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof WorkspaceRail>;

export const Default: Story = {
  args: {
    workspaces: [sampleWorkspace, personalWorkspace],
    activeWorkspace: sampleWorkspace,
    onSelectWorkspace: fn(),
    onCreateWorkspace: fn(),
    user: sampleUser,
    onAvatarClick: fn(),
  },
};

export const SingleWorkspace: Story = {
  args: {
    workspaces: [personalWorkspace],
    activeWorkspace: personalWorkspace,
    onSelectWorkspace: fn(),
    onCreateWorkspace: fn(),
    user: sampleUser,
  },
};

export const NoUser: Story = {
  args: {
    workspaces: [sampleWorkspace],
    activeWorkspace: sampleWorkspace,
    onSelectWorkspace: fn(),
    onCreateWorkspace: fn(),
    user: null,
  },
};
