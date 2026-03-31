import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ProjectSetupPage } from "../components/ProjectSetupPage";

const meta: Meta<typeof ProjectSetupPage> = {
  title: "Pages/ProjectSetupPage",
  component: ProjectSetupPage,
  tags: ["autodocs"],
  args: {
    tabID: "story-tab",
    onDone: fn(),
  },
  parameters: {
    layout: "centered",
  },
};

export default meta;
type Story = StoryObj<typeof ProjectSetupPage>;

export const Default: Story = {};
