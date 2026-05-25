import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ToolPalette } from "../ToolPalette";
import type { ToolInfo } from "../types";
import toolsData from "../../../../apps/kapi-desktop/frontend/src/stories/fixtures/tools-metadata.json";

const tools = toolsData as ToolInfo[];

const meta: Meta<typeof ToolPalette> = {
  title: "Flow Editor/ToolPalette",
  component: ToolPalette,
  tags: ["autodocs"],
  args: {
    onAddTool: fn(),
  },
  parameters: { layout: "fullscreen" },
  decorators: [
    (Story) => (
      <div style={{ height: 600, display: "flex" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ToolPalette>;

export const AllTools: Story = {
  args: { tools },
};

export const BuiltInOnly: Story = {
  args: {
    tools: tools.filter((t) => !t.name.startsWith("okapi:")),
  },
};

export const OkapiOnly: Story = {
  args: {
    tools: tools.filter((t) => t.name.startsWith("okapi:")),
  },
};

export const FewTools: Story = {
  args: {
    tools: tools.slice(0, 8),
  },
};

// ---------------------------------------------------------------------------
// Source-transform badge stories
// ---------------------------------------------------------------------------

const sourceTransformTools: ToolInfo[] = [
  {
    name: "redact",
    display_name: "Redact",
    description: "Replace sensitive spans with placeholders before translation",
    category: "transform",
    has_schema: true,
    inputs: ["block"],
    tags: ["privacy", "pre-processing"],
    isSourceTransform: true,
  },
  {
    name: "source-normalise",
    display_name: "Source Normalise",
    description: "Normalise quotes, punctuation, and whitespace in source text",
    category: "transform",
    has_schema: true,
    inputs: ["block"],
    tags: ["text-processing"],
    isSourceTransform: true,
  },
  {
    name: "source-simplifier",
    display_name: "Source Simplifier",
    description: "Simplify complex source sentences to aid machine translation",
    category: "transform",
    has_schema: false,
    inputs: ["block"],
    tags: ["ai-powered"],
    isSourceTransform: true,
  },
  // Ordinary tools without source-transform capability
  {
    name: "ai-translate",
    display_name: "AI Translate",
    description: "Translate content using AI/LLM",
    category: "translate",
    has_schema: false,
    inputs: ["block"],
    tags: ["ai-powered", "translation"],
  },
  {
    name: "qa-check",
    display_name: "QA Check",
    description: "Run rule-based quality checks",
    category: "validate",
    has_schema: true,
    inputs: ["block"],
    tags: ["quality"],
  },
];

/**
 * Palette with source-transform-capable tools showing the blue "pre" badge
 * next to the tool name. Non-capable tools show no badge.
 */
export const WithSourceTransformBadges: Story = {
  name: "Source-Transform Badges",
  args: {
    tools: sourceTransformTools,
  },
};
