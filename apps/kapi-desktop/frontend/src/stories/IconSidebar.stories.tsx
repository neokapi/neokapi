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

// A project open on its Overview.
export const ProjectMode: Story = {
  args: {
    active: "map",
  },
};

// Project Settings on its Assistants tab keeps the Project Settings entry lit.
export const SettingsAssistants: Story = {
  args: {
    active: "project-settings:assistants",
  },
};

export const ProjectDisabled: Story = {
  args: {
    active: "home",
    projectDisabled: true,
  },
};
