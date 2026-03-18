import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { WorkspaceIcon } from "../../components/WorkspaceIcon";
import { sampleWorkspace, personalWorkspace } from "./fixtures";

const meta: Meta<typeof WorkspaceIcon> = {
  title: "Components/WorkspaceIcon",
  component: WorkspaceIcon,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ padding: 24, display: "flex", gap: 16, alignItems: "center" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof WorkspaceIcon>;

export const Active: Story = {
  args: { workspace: sampleWorkspace, active: true, onClick: fn() },
};

export const Inactive: Story = {
  args: { workspace: personalWorkspace, active: false, onClick: fn() },
};

export const Large: Story = {
  args: { workspace: sampleWorkspace, active: true, onClick: fn(), size: 56 },
};
