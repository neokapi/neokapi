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

// A single-language project: the content surfaces show, Review is absent,
// and Languages + is the one door to language work.
export const ProjectMode: Story = {
  args: {
    active: "project-home",
    hasTargetLanguages: false,
  },
};

// The Languages + door open, before the recipe declares a target language.
export const LanguagesDoor: Story = {
  args: {
    active: "languages",
    hasTargetLanguages: false,
  },
};

// Once the recipe declares a target language the entry reads Languages and
// Review returns beside it.
export const ProjectMultilingual: Story = {
  args: {
    active: "languages",
    hasTargetLanguages: true,
  },
};

export const ProjectDisabled: Story = {
  args: {
    active: "home",
    projectDisabled: true,
  },
};
