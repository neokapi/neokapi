import type { Meta, StoryObj } from "@storybook/react-vite";
import { BlockInspector } from "@neokapi/ui-primitives/preview";
import { plainBlock, richBlock } from "./mockData";

const meta: Meta<typeof BlockInspector> = {
  title: "Lab/Block Inspector",
  component: BlockInspector,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof BlockInspector>;

export const Rich: Story = {
  name: "Full detail (targets, overlays, annotations)",
  args: { node: richBlock, defaultOpen: true },
};

export const Collapsed: Story = {
  args: { node: richBlock, defaultOpen: false },
};

export const Plain: Story = {
  name: "Source only",
  args: { node: plainBlock, defaultOpen: true },
};

export const Changed: Story = {
  name: "Changed by a run",
  args: { node: richBlock, defaultOpen: true, changed: true },
};
