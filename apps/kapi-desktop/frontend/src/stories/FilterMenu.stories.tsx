import type { Meta, StoryObj } from "@storybook/react-vite";
import { FilterMenu } from "../components/FilterMenu";
import type { KapiProject } from "../types/api";

const project: KapiProject = {
  version: "v1",
  name: "Acme App",
  defaults: {
    source_language: "en-US",
    target_languages: ["fr-FR", "de-DE", "ja-JP", "nb-NO"],
  },
  content: [
    { name: "Website", items: [{ path: "docs/**/*.md" }] },
    { name: "Online Store", items: [{ path: "store/*.json" }] },
    { name: "Contracts", items: [{ path: "contracts/*.docx" }] },
  ],
};

const meta: Meta<typeof FilterMenu> = {
  title: "Components/FilterMenu",
  component: FilterMenu,
  tags: ["autodocs"],
  args: { project },
  parameters: { layout: "centered" },
};

export default meta;
type Story = StoryObj<typeof FilterMenu>;

/**
 * The menu-bar Active Filter control. Without a backing project the dropdown
 * shows "All" and "New filter…"; opening "New filter…" reveals the editor
 * populated from the project's collections and target languages.
 */
export const Default: Story = {};
