import type { Meta, StoryObj } from "@storybook/react-vite";
import { ProjectFilesPage } from "../components/ProjectFilesPage";

const meta: Meta<typeof ProjectFilesPage> = {
  title: "Pages/ProjectFilesPage",
  component: ProjectFilesPage,
  tags: ["autodocs"],
  args: {
    tabID: "story-tab",
    basePath: "/Users/dev/acme-app",
  },
};

export default meta;
type Story = StoryObj<typeof ProjectFilesPage>;

export const Default: Story = {};
