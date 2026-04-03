import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { IconSidebar } from "../components/IconSidebar";

const meta: Meta<typeof IconSidebar> = {
  title: "Components/IconSidebar",
  component: IconSidebar,
  tags: ["autodocs"],
  args: {
    onChange: fn(),
  },
  decorators: [
    (Story) => (
      <div style={{ height: 500, display: "flex" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof IconSidebar>;

export const AdhocMode: Story = {
  args: {
    mode: "adhoc",
    active: "tools",
  },
};

export const ProjectMode: Story = {
  args: {
    mode: "projects",
    active: "project-home",
  },
};

export const ProjectDisabled: Story = {
  args: {
    mode: "projects",
    active: "home",
    projectDisabled: true,
  },
};
