import type { Meta, StoryObj } from "@storybook/react-vite";
import { Separator } from "@neokapi/ui-primitives/components/ui/separator";

const meta: Meta<typeof Separator> = {
  title: "Foundations/Separator",
  component: Separator,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 400, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof Separator>;

export const Horizontal: Story = {
  render: () => (
    <div>
      <div className="space-y-1">
        <h4 className="text-sm font-medium leading-none">Translation Memory</h4>
        <p className="text-sm text-muted-foreground">Reuse past translations across projects.</p>
      </div>
      <Separator className="my-4" />
      <div className="space-y-1">
        <h4 className="text-sm font-medium leading-none">Terminology</h4>
        <p className="text-sm text-muted-foreground">Manage approved terms and glossaries.</p>
      </div>
    </div>
  ),
};

export const Vertical: Story = {
  render: () => (
    <div className="flex h-5 items-center space-x-4 text-sm">
      <span>Source</span>
      <Separator orientation="vertical" />
      <span>Target</span>
      <Separator orientation="vertical" />
      <span>Preview</span>
    </div>
  ),
};
