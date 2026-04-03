import type { Meta, StoryObj } from "@storybook/react-vite";
import { BravoColdStart } from "../../components/bravo/BravoColdStart";

const meta: Meta<typeof BravoColdStart> = {
  title: "Bravo/BravoColdStart",
  component: BravoColdStart,
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
type Story = StoryObj<typeof BravoColdStart>;

export const Default: Story = {};
