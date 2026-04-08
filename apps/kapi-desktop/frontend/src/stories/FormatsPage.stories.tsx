import type { Meta, StoryObj } from "@storybook/react-vite";
import { FormatsPage } from "../components/FormatsPage";
import pluginDocs from "./fixtures/plugin-docs.json";
import formatList from "./fixtures/format-list.json";
import formatSchemas from "./fixtures/format-schemas.json";
import presetsData from "./fixtures/presets.json";
import type { FormatInfo, PluginDocs } from "../types/api";
import type { ComponentSchema } from "@neokapi/ui-primitives";

const docs = pluginDocs as unknown as PluginDocs;
const formats = [
  ...formatList.builtIn,
  ...formatList.bridge.map((f) => ({ ...f, source: "okapi-bridge" })),
] as FormatInfo[];

// Build schema lookup by format name
type SE = ComponentSchema & { "x-name": string };
const allSchemas = [...formatSchemas.builtIn, ...formatSchemas.bridge] as unknown as SE[];
const schemas: Record<string, ComponentSchema> = {};
for (const s of allSchemas) {
  schemas[s["x-name"]] = s;
}

// Build presets lookup by format name
const presets: Record<
  string,
  Array<{
    name: string;
    description: string;
    format: string;
    config?: Record<string, unknown>;
    source?: string;
  }>
> = {};
for (const [formatId, formatPresets] of Object.entries(presetsData)) {
  presets[formatId] = Object.entries(formatPresets as Record<string, unknown>)
    .filter(([, v]) => v != null)
    .map(([name, config]) => ({
      name,
      description: "",
      format: formatId,
      config: config as Record<string, unknown>,
      source: "built-in",
    }));
}

const meta: Meta<typeof FormatsPage> = {
  title: "Pages/FormatsPage",
  component: FormatsPage,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof FormatsPage>;

/** Loading state showing skeleton format cards. */
export const Loading: Story = {
  args: {
    formats: [],
    forceLoading: true,
  },
};

/**
 * Default view with all formats, schemas, presets, and documentation.
 */
export const Default: Story = {
  args: {
    formats,
    docs,
    schemas,
    presets,
  },
};

/**
 * Built-in formats only (no plugins).
 */
export const BuiltInOnly: Story = {
  name: "Built-in Only",
  args: {
    formats: formatList.builtIn as FormatInfo[],
    schemas,
    presets,
  },
};

/**
 * Plugin formats with documentation.
 */
export const PluginFormats: Story = {
  name: "Plugin Formats",
  args: {
    formats: formatList.bridge.map((f) => ({
      ...f,
      source: "okapi-bridge",
    })) as unknown as FormatInfo[],
    docs,
    schemas,
    presets,
  },
};
