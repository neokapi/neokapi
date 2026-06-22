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

// A single-language project: the content surfaces show; Translation Memories
// stays absent until the project has target languages.
export const ProjectMode: Story = {
  args: {
    mode: "projects",
    active: "project-home",
    hasTargetLanguages: false,
  },
};

// Once the project has target languages, Translation Memories appears — present,
// not announced.
export const ProjectMultilingual: Story = {
  args: {
    mode: "projects",
    active: "memories",
    hasTargetLanguages: true,
  },
};

export const ProjectDisabled: Story = {
  args: {
    mode: "projects",
    active: "home",
    projectDisabled: true,
  },
};
