import type { Meta, StoryObj } from "@storybook/react-vite";
import { ContentTreeView } from "@neokapi/ui-primitives/preview";
import { mockTree } from "./mockData";

const meta: Meta<typeof ContentTreeView> = {
  title: "Lab/Content Tree",
  component: ContentTreeView,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof ContentTreeView>;

export const Default: Story = {
  args: { tree: mockTree },
};

export const BlocksExpanded: Story = {
  args: { tree: mockTree, expandBlocks: true },
};

export const WithChangedBlock: Story = {
  name: "With a changed block",
  args: { tree: mockTree, changedIds: new Set(["greeting"]) },
};
