import type { Meta, StoryObj } from "@storybook/react-vite";
import { FormatsPage } from "../components/FormatsPage";

const meta: Meta<typeof FormatsPage> = {
  title: "Pages/FormatsPage",
  component: FormatsPage,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof FormatsPage>;

export const Default: Story = {};
