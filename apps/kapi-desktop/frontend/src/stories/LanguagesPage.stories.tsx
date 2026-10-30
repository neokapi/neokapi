import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { LanguagesPage } from "../components/LanguagesPage";

const meta: Meta<typeof LanguagesPage> = {
  title: "Pages/LanguagesPage",
  component: LanguagesPage,
  tags: ["autodocs"],
  args: {
    onUpdate: fn(),
  },
};

export default meta;
type Story = StoryObj<typeof LanguagesPage>;

/** What the rail's Languages + opens on a project with one language. */
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
