import type { Meta, StoryObj } from "@storybook/react-vite";
import { Label } from "../../components/ui/label";
import { Input } from "../../components/ui/input";

const meta: Meta<typeof Label> = {
  title: "Foundations/Label",
  component: Label,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 300, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof Label>;

export const Default: Story = {
  args: { children: "Source Language" },
};

export const WithInput: Story = {
  render: () => (
    <div className="grid w-full gap-1.5">
      <Label htmlFor="project-name">Project Name</Label>
      <Input id="project-name" placeholder="My Localization Project" />
    </div>
  ),
};

export const Disabled: Story = {
  render: () => (
    <div className="grid w-full gap-1.5" data-disabled="true">
      <Label>Disabled Label</Label>
      <Input disabled placeholder="Disabled input" />
    </div>
  ),
};
