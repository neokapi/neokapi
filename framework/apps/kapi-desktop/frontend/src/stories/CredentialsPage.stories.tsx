import type { Meta, StoryObj } from "@storybook/react-vite";
import { CredentialsPage } from "../components/CredentialsPage";

const meta: Meta<typeof CredentialsPage> = {
  title: "Pages/CredentialsPage",
  component: CredentialsPage,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof CredentialsPage>;

export const Default: Story = {};
