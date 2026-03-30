import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { FlowEditor } from "../FlowEditor";
import type { ToolInfo, ComponentSchema } from "../types";
import toolsData from "../../../../framework/apps/kapi-desktop/frontend/src/stories/fixtures/tools-metadata.json";

const tools = toolsData as ToolInfo[];

const sampleSchemas: Record<string, ComponentSchema> = {
  "pseudo-translate": {
    title: "Pseudo Translate",
    type: "object",
    "x-component": { id: "pseudo-translate", type: "tool", category: "transform" },
    "x-groups": [
      { id: "output", label: "Output Format", fields: ["prefix", "suffix", "expansionPercent"] },
    ],
    properties: {
      prefix: { type: "string", default: "[", description: "Prefix added to translations" },
      suffix: { type: "string", default: "]", description: "Suffix added to translations" },
      expansionPercent: {
        type: "integer",
        default: 30,
        minimum: 0,
        maximum: 200,
        description: "Expand text length %",
      },
      applyAccents: {
        type: "boolean",
        default: true,
        description: "Apply diacritical marks to characters",
      },
      padWithX: {
        type: "boolean",
        default: false,
        description: "Pad expansion with 'x' characters",
      },
    },
  },
  "qa-check": {
    title: "QA Check",
    type: "object",
    "x-component": { id: "qa-check", type: "tool", category: "validate" },
    "x-groups": [
      {
        id: "checks",
        label: "Enabled Checks",
        fields: [
          "checkLeadingWhitespace",
          "checkTrailingWhitespace",
          "checkDoubleSpaces",
          "checkMissingTranslation",
        ],
      },
      { id: "codes", label: "Code Checks", fields: ["checkInlineCodes", "checkPatterns"] },
    ],
    properties: {
      checkLeadingWhitespace: {
        type: "boolean",
        default: true,
        description: "Check for leading whitespace mismatches",
      },
      checkTrailingWhitespace: {
        type: "boolean",
        default: true,
        description: "Check trailing whitespace",
      },
      checkDoubleSpaces: {
        type: "boolean",
        default: true,
        description: "Flag double spaces in target",
      },
      checkMissingTranslation: {
        type: "boolean",
        default: true,
        description: "Flag empty translations",
      },
      checkInlineCodes: {
        type: "boolean",
        default: true,
        description: "Verify inline codes are preserved",
      },
      checkPatterns: {
        type: "boolean",
        default: false,
        description: "Check for pattern mismatches",
      },
      severityLevel: {
        type: "string",
        default: "warning",
        enum: ["error", "warning", "info"],
        description: "Default severity",
      },
    },
  },
  "search-replace": {
    title: "Search and Replace",
    type: "object",
    "x-component": { id: "search-replace", type: "tool", category: "transform" },
    properties: {
      search: { type: "string", description: "Search pattern" },
      replace: { type: "string", description: "Replacement text" },
      regEx: { type: "boolean", default: false, description: "Use regular expressions" },
      target: { type: "boolean", default: true, description: "Apply to target text" },
      source: { type: "boolean", default: false, description: "Apply to source text" },
      dotAll: { type: "boolean", default: false, description: "Dot matches newlines" },
    },
  },
};

function getSchema(toolName: string): ComponentSchema | null {
  return sampleSchemas[toolName] || null;
}

const meta: Meta<typeof FlowEditor> = {
  title: "Flow Editor/FlowEditor",
  component: FlowEditor,
  tags: ["autodocs"],
  args: {
    onChange: fn(),
    onRun: fn(),
    onGetSchema: getSchema,
  },
  parameters: { layout: "fullscreen" },
  decorators: [
    (Story) => (
      <div style={{ height: 700 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof FlowEditor>;

export const SingleStep: Story = {
  args: {
    flow: { steps: [{ tool: "ai-translate" }] },
    tools,
  },
};

export const MultiStep: Story = {
  args: {
    flow: {
      steps: [{ tool: "ai-translate" }, { tool: "qa-check" }],
    },
    tools,
  },
};

export const FullPipeline: Story = {
  args: {
    flow: {
      steps: [
        { tool: "tm-leverage" },
        { tool: "ai-translate" },
        { tool: "pseudo-translate", config: { prefix: ">>", suffix: "<<" } },
        { tool: "qa-check" },
        { tool: "word-count" },
      ],
    },
    tools,
  },
};

export const WithOkapiTools: Story = {
  args: {
    flow: {
      steps: [
        { tool: "okapi:segmentation" },
        { tool: "okapi:leveraging" },
        { tool: "okapi:quality-check" },
      ],
    },
    tools,
  },
};

export const EmptyWithTemplates: Story = {
  name: "Empty (Template Library)",
  args: {
    flow: { steps: [] },
    tools,
  },
};

export const ReadOnly: Story = {
  args: {
    flow: {
      steps: [{ tool: "ai-translate" }, { tool: "qa-check" }],
    },
    tools,
    readOnly: true,
    onRun: undefined,
  },
};

export const WithConfiguration: Story = {
  args: {
    flow: {
      steps: [
        { tool: "pseudo-translate", config: { prefix: ">>", suffix: "<<", expansionPercent: 40 } },
        { tool: "qa-check", config: { checkLeadingWhitespace: false } },
        { tool: "search-replace", config: { search: "foo", replace: "bar", regEx: false } },
      ],
    },
    tools,
  },
};

export const ParallelBranches: Story = {
  args: {
    flow: {
      steps: [
        { tool: "ai-translate", label: "Translate" },
        {
          tool: "",
          parallel: [
            { tool: "qa-check", label: "Quality Check" },
            { tool: "brand-vocab-check", label: "Brand Check" },
          ],
        },
        { tool: "word-count", label: "Word Count" },
      ],
    },
    tools,
  },
};

export const ThreeWayParallel: Story = {
  args: {
    flow: {
      steps: [
        { tool: "tm-leverage", label: "TM Lookup" },
        {
          tool: "",
          parallel: [
            { tool: "qa-check", label: "QA" },
            { tool: "brand-vocab-check", label: "Brand" },
            { tool: "entity-extract", label: "Entities" },
          ],
        },
      ],
    },
    tools,
  },
};

export const ParallelizationSuggestion: Story = {
  name: "Parallelization Suggestion",
  args: {
    flow: {
      steps: [
        { tool: "ai-translate" },
        { tool: "qa-check" },
        { tool: "brand-vocab-check" },
        { tool: "word-count" },
      ],
    },
    tools,
  },
};

export const WithPortVisualization: Story = {
  name: "With Port Visualization",
  args: {
    flow: {
      steps: [{ tool: "ai-translate" }, { tool: "qa-check" }, { tool: "word-count" }],
    },
    tools: tools.map((t) => ({
      ...t,
      inputs:
        t.name === "ai-translate"
          ? ["block"]
          : t.name === "qa-check"
            ? ["block"]
            : ["block", "data"],
      outputs: t.name === "ai-translate" ? ["block"] : t.name === "qa-check" ? ["block"] : ["data"],
    })),
  },
};

export const WithTraceData: Story = {
  name: "With Trace (Completed)",
  args: {
    flow: {
      steps: [{ tool: "ai-translate" }, { tool: "qa-check" }, { tool: "word-count" }],
    },
    tools,
    readOnly: true,
    onRun: undefined,
    traceEvents: [
      { ts: 0, type: "enter", nodeId: "tool-0", partId: "p1" },
      { ts: 500, type: "exit", nodeId: "tool-0", partId: "p1" },
      { ts: 600, type: "enter", nodeId: "tool-0", partId: "p2" },
      { ts: 900, type: "exit", nodeId: "tool-0", partId: "p2" },
      { ts: 550, type: "enter", nodeId: "tool-1", partId: "p1" },
      { ts: 1200, type: "exit", nodeId: "tool-1", partId: "p1" },
      { ts: 950, type: "enter", nodeId: "tool-1", partId: "p2" },
      { ts: 1800, type: "exit", nodeId: "tool-1", partId: "p2" },
      { ts: 1250, type: "enter", nodeId: "tool-2", partId: "p1" },
      { ts: 1400, type: "exit", nodeId: "tool-2", partId: "p1" },
      { ts: 1850, type: "enter", nodeId: "tool-2", partId: "p2" },
      { ts: 2000, type: "exit", nodeId: "tool-2", partId: "p2" },
    ],
    trace: {
      name: "translate-qa",
      nodes: [
        { id: "tool-0", type: "tool", name: "ai-translate" },
        { id: "tool-1", type: "tool", name: "qa-check" },
        { id: "tool-2", type: "tool", name: "word-count" },
      ],
      events: [],
      parts: {
        p1: {
          initial: { id: "p1", type: "Block", summary: "Hello world", sourceText: "Hello world" },
          afterNode: {
            "tool-0": {
              id: "p1",
              type: "Block",
              summary: "Hello world",
              sourceText: "Hello world",
              targetText: "Bonjour le monde",
            },
            "tool-1": {
              id: "p1",
              type: "Block",
              summary: "Hello world",
              sourceText: "Hello world",
              targetText: "Bonjour le monde",
            },
            "tool-2": {
              id: "p1",
              type: "Block",
              summary: "Hello world",
              sourceText: "Hello world",
              targetText: "Bonjour le monde",
            },
          },
        },
        p2: {
          initial: { id: "p2", type: "Block", summary: "Click here", sourceText: "Click here" },
          afterNode: {
            "tool-0": {
              id: "p2",
              type: "Block",
              summary: "Click here",
              sourceText: "Click here",
              targetText: "Cliquez ici",
            },
            "tool-1": {
              id: "p2",
              type: "Block",
              summary: "Click here",
              sourceText: "Click here",
              targetText: "Cliquez ici",
            },
            "tool-2": {
              id: "p2",
              type: "Block",
              summary: "Click here",
              sourceText: "Click here",
              targetText: "Cliquez ici",
            },
          },
        },
      },
      durationUs: 2000,
    },
  },
};
