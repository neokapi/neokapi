import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ModeToggle } from "../components/ModeToggle";

const meta: Meta<typeof ModeToggle> = {
  title: "Components/ModeToggle",
  component: ModeToggle,
  tags: ["autodocs"],
  args: {
    onChange: fn(),
  },
};

export default meta;
type Story = StoryObj<typeof ModeToggle>;

export const AdhocMode: Story = {
  args: {
    mode: "adhoc",
  },
};

export const ProjectMode: Story = {
  args: {
    mode: "projects",
  },
};
