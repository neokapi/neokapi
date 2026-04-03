import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { Sidebar } from "../components/Sidebar";

const meta: Meta<typeof Sidebar> = {
  title: "Components/Sidebar",
  component: Sidebar,
  tags: ["autodocs"],
  args: {
    onViewChange: fn(),
    onCloseProject: fn(),
  },
  decorators: [
    (Story) => (
      <div style={{ height: 600, display: "flex" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof Sidebar>;

export const Default: Story = {
  args: {
    activeView: "project",
    projectName: "Acme App Localization",
  },
};

export const FlowsActive: Story = {
  args: {
    activeView: "flows",
    projectName: "My Project",
  },
};

export const NoProject: Story = {
  args: {
    activeView: "settings",
  },
};
