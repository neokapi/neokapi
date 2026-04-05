import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ProjectPresetPage } from "../components/ProjectPresetPage";

const meta: Meta<typeof ProjectPresetPage> = {
  title: "Pages/ProjectPresetPage",
  component: ProjectPresetPage,
  tags: ["autodocs"],
  args: {
    tabID: "story-tab",
    onApplied: fn(),
    onSkip: fn(),
  },
  parameters: {
    layout: "centered",
  },
};

export default meta;
type Story = StoryObj<typeof ProjectPresetPage>;

export const DetectedNextjs: Story = {
  args: {
    detectedPreset: "nextjs",
  },
};

export const DetectedAngular: Story = {
  args: {
    detectedPreset: "angular",
  },
};
