import type { Meta, StoryObj } from "@storybook/react-vite";
import { ToolRunnerPage } from "../components/ToolRunnerPage";
import { ErrorProvider } from "../components/ErrorBanner";
import { pluginDocs, toolsMetadata } from "./_lib/reference-data";
import type { PluginDocs, ToolInfo } from "../types/api";

const docs = pluginDocs as unknown as PluginDocs;
const tools = toolsMetadata as unknown as ToolInfo[];

// Add some Okapi step tools that match docs
const okapiTools: ToolInfo[] = Object.entries(docs.steps).map(([name, doc]) => ({
  name,
  description: (doc as { overview: string }).overview.slice(0, 80) + "...",
  category: name.includes("translation")
    ? "translate"
    : name.includes("count") || name.includes("character")
      ? "validate"
      : name.includes("search")
        ? "transform"
        : "pipeline",
  has_schema: true,
  inputs: ["block"],
  tags: ["okapi"],
  requires: [],
}));

const allTools = [...tools, ...okapiTools];

const meta: Meta<typeof ToolRunnerPage> = {
  title: "Pages/ToolRunnerPage",
  component: ToolRunnerPage,
  tags: ["autodocs"],
  parameters: {
    layout: "fullscreen",
  },
  decorators: [
    (Story) => (
      <ErrorProvider>
        <div style={{ height: 700 }}>
          <Story />
        </div>
      </ErrorProvider>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ToolRunnerPage>;

/**
 * Default view — empty, fetches from backend (null in Storybook).
 */
export const Default: Story = {};

/**
 * Tool browser with pre-loaded tools and documentation.
 * Shows the enhanced categorized list with doc panels.
 */
export const WithDocsAndTools: Story = {
  name: "With Docs & Tools",
  args: {
    docs,
    tools: allTools,
  },
};

/**
 * Built-in tools only (no Okapi docs).
 */
export const BuiltInOnly: Story = {
  name: "Built-in Tools Only",
  args: {
    tools,
  },
};

/**
 * Okapi pipeline steps with full documentation.
 */
export const OkapiStepsOnly: Story = {
  name: "Okapi Steps Only",
  args: {
    docs,
    tools: okapiTools,
  },
};
