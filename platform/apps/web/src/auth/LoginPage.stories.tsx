import type { Meta, StoryObj } from "@storybook/react-vite";
import { LoginPage } from "./LoginPage";

const meta: Meta<typeof LoginPage> = {
  title: "Auth/Web/Login",
  component: LoginPage,
  parameters: {
    layout: "fullscreen",
  },
};

export default meta;
type Story = StoryObj<typeof LoginPage>;

export const Default: Story = {};
