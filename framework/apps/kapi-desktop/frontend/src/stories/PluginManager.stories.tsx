import type { Meta, StoryObj } from "@storybook/react-vite";
import { PluginManager } from "../components/PluginManager";

const meta: Meta<typeof PluginManager> = {
  title: "Pages/PluginManager",
  component: PluginManager,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof PluginManager>;

export const Default: Story = {};
