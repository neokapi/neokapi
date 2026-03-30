import type { Meta, StoryObj } from "@storybook/react-vite";
import { MemoriesPage } from "../components/MemoriesPage";

const meta: Meta<typeof MemoriesPage> = {
  title: "Pages/MemoriesPage",
  component: MemoriesPage,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof MemoriesPage>;

export const Default: Story = {};
