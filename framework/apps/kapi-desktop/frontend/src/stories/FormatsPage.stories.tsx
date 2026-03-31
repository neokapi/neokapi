import type { Meta, StoryObj } from "@storybook/react-vite";
import { FormatsPage } from "../components/FormatsPage";
import pluginDocs from "./fixtures/plugin-docs.json";
import type { PluginDocs } from "../types/api";

const docs = pluginDocs as unknown as PluginDocs;

const meta: Meta<typeof FormatsPage> = {
  title: "Pages/FormatsPage",
  component: FormatsPage,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof FormatsPage>;

/**
 * Default view — the format list with both built-in and plugin formats.
 * In Storybook the API returns null so formats are empty.
 * Use the WithDocs story for rich simulated data.
 */
export const Default: Story = {};

/**
 * Formats page with pre-loaded documentation data.
 * Shows the enhanced cards with doc overview snippets.
 */
export const WithDocs: Story = {
  args: {
    docs,
  },
};
