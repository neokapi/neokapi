import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ReviewSetupPage } from "../components/ReviewSetupPage";

const meta: Meta<typeof ReviewSetupPage> = {
  title: "Pages/ReviewSetupPage",
  component: ReviewSetupPage,
  tags: ["autodocs"],
  args: {
    onUpdate: fn(),
  },
};

export default meta;
type Story = StoryObj<typeof ReviewSetupPage>;

/** What Review shows on a project with one language. */
export const SourceOnly: Story = {
  args: {
    project: {
      version: "v1",
      name: "Handbook",
      defaults: { source_language: "en-US" },
      collections: [{ name: "Handbook", content: [{ path: "docs/**/*.md" }] }],
    },
  },
};

/** No source language set yet either. */
export const NothingSet: Story = {
  args: {
    project: {
      version: "v1",
      name: "Handbook",
      collections: [{ name: "Handbook", content: [{ path: "docs/**/*.md" }] }],
    },
  },
};
