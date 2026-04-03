import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { EntityMarkPopover } from "../../components/editor/EntityMarkPopover";

const meta: Meta<typeof EntityMarkPopover> = {
  title: "Editor/Entities/EntityMarkPopover",
  component: EntityMarkPopover,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ position: "relative", padding: 80 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof EntityMarkPopover>;

/** Mark a person entity. */
export const Default: Story = {
  args: {
    text: "John Smith",
    start: 0,
    end: 10,
    position: { x: 100, y: 40 },
    onConfirm: fn(),
    onCancel: fn(),
  },
};

/** Mark a longer organization name. */
export const Organization: Story = {
  args: {
    text: "Acme Corporation International",
    start: 15,
    end: 45,
    position: { x: 200, y: 60 },
    onConfirm: fn(),
    onCancel: fn(),
  },
};
