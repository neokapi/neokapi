import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { WelcomePage } from "../components/WelcomePage";

const meta: Meta<typeof WelcomePage> = {
  title: "Pages/WelcomePage",
  component: WelcomePage,
  tags: ["autodocs"],
  args: {
    onOpen: fn(),
    onNew: fn(),
  },
  parameters: {
    layout: "fullscreen",
  },
};

export default meta;
type Story = StoryObj<typeof WelcomePage>;

export const Default: Story = {};

export const DarkTheme: Story = {
  parameters: {
    themes: { themeOverride: "dark" },
  },
};

export const LightTheme: Story = {
  parameters: {
    themes: { themeOverride: "light" },
  },
};
