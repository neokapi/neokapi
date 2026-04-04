import { ToggleGroup, ToggleGroupItem } from "@neokapi/ui-primitives";
import type { Meta, StoryObj } from "@storybook/react-vite";

const meta: Meta<typeof ToggleGroup> = {
  title: "Foundations/ToggleGroup",
  component: ToggleGroup,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ToggleGroup>;

export const Default: Story = {
  render: () => (
    <ToggleGroup type="single" defaultValue="source">
      <ToggleGroupItem value="source">Source</ToggleGroupItem>
      <ToggleGroupItem value="target">Target</ToggleGroupItem>
      <ToggleGroupItem value="preview">Preview</ToggleGroupItem>
    </ToggleGroup>
  ),
};

export const Multiple: Story = {
  render: () => (
    <ToggleGroup type="multiple" defaultValue={["source", "target"]}>
      <ToggleGroupItem value="source">Source</ToggleGroupItem>
      <ToggleGroupItem value="target">Target</ToggleGroupItem>
      <ToggleGroupItem value="comments">Comments</ToggleGroupItem>
      <ToggleGroupItem value="metadata">Metadata</ToggleGroupItem>
    </ToggleGroup>
  ),
};
