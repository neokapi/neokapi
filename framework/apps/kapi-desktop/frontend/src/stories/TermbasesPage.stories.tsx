import type { Meta, StoryObj } from "@storybook/react-vite";
import { TermbasesPage } from "../components/TermbasesPage";

const meta: Meta<typeof TermbasesPage> = {
  title: "Pages/TermbasesPage",
  component: TermbasesPage,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof TermbasesPage>;

export const Default: Story = {};
