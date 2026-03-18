import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { BravoComposer } from "../../components/bravo/BravoComposer";

const meta: Meta<typeof BravoComposer> = {
  title: "Bravo/BravoComposer",
  component: BravoComposer,
  tags: ["autodocs"],
  decorators: [(Story) => <div style={{ maxWidth: 480, padding: 0 }}><Story /></div>],
};

export default meta;
type Story = StoryObj<typeof BravoComposer>;

export const Default: Story = {
  args: {
    onSend: fn(),
  },
};

export const Disabled: Story = {
  args: {
    onSend: fn(),
    disabled: true,
  },
};

export const CustomPlaceholder: Story = {
  args: {
    onSend: fn(),
    placeholder: "Ask @bravo to translate your files...",
  },
};
