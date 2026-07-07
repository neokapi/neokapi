import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { WorkspaceTable } from "./WorkspaceTable";
import { sampleWorkspaces } from "../ctrl-fixtures";

const meta: Meta<typeof WorkspaceTable> = {
  title: "Ctrl/WorkspaceTable",
  component: WorkspaceTable,
  parameters: { layout: "padded" },
  args: { onRowClick: fn() },
  decorators: [
    (Story) => (
      <div className="w-full max-w-5xl">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof WorkspaceTable>;

export const Default: Story = {
  args: { workspaces: sampleWorkspaces, loading: false },
};

export const Loading: Story = {
  args: { workspaces: [], loading: true },
};

export const Empty: Story = {
  args: { workspaces: [], loading: false },
};
