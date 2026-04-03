import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { StarterPackPicker } from "../../brand/StarterPackPicker";

const meta: Meta<typeof StarterPackPicker> = {
  title: "Brand/StarterPackPicker",
  component: StarterPackPicker,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof StarterPackPicker>;

/** Dialog open with all starter packs and a "Start from Scratch" option. */
export const Open: Story = {
  args: {
    open: true,
    onOpenChange: fn(),
    onSelect: fn(),
    onScratch: fn(),
  },
};
