import { Toggle } from "@neokapi/ui-primitives";
import type { Meta, StoryObj } from "@storybook/react-vite";

const meta: Meta<typeof Toggle> = {
  title: "Foundations/Toggle",
  component: Toggle,
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
type Story = StoryObj<typeof Toggle>;

export const Default: Story = {
  render: () => (
    <div className="flex items-center gap-2">
      <Toggle aria-label="Toggle bold">
        <span className="font-bold">B</span>
      </Toggle>
      <Toggle aria-label="Toggle italic">
        <span className="italic">I</span>
      </Toggle>
      <Toggle aria-label="Toggle underline">
        <span className="underline">U</span>
      </Toggle>
    </div>
  ),
};
