import type { Meta, StoryObj } from "@storybook/react-vite";
import { SettingsPage } from "../components/SettingsPage";

const meta: Meta<typeof SettingsPage> = {
  title: "Pages/SettingsPage",
  component: SettingsPage,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof SettingsPage>;

export const Default: Story = {};
