import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { FlowEditor } from "../FlowEditor";
import type { ToolInfo, ToolDoc, ComponentSchema } from "../types";
import toolsData from "../../../../apps/kapi-desktop/frontend/src/stories/fixtures/tools-metadata.json";

const tools = toolsData as ToolInfo[];

const sampleSchemas: Record<string, ComponentSchema> = {
  "pseudo-translate": {
    title: "Pseudo Translate",
    type: "object",
    toolMeta: { id: "pseudo-translate", category: "transform" },
    "ui:groups": [
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
    toolMeta: { id: "qa-check", category: "validate" },
    "ui:groups": [
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
    toolMeta: { id: "search-replace", category: "transform" },
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

const sampleDocs: Record<string, ToolDoc> = {
  "pseudo-translate": {
    displayName: "Pseudo Translation",
    overview:
      "Generates pseudo-translations by applying diacritical marks, padding, and brackets to source text. Useful for testing UI layout, detecting hardcoded strings, and verifying internationalization readiness without real translations.",
    parameters: {
      prefix: {
        description:
          "Character(s) prepended to each translated string. Helps identify translated vs untranslated strings in the UI.",
      },
      suffix: { description: "Character(s) appended to each translated string." },
      expansionPercent: {
        description:
          "Percentage to expand text length to simulate longer translations (e.g. German is ~30% longer than English).",
        notes: ["Set to 0 to disable expansion. Values above 100% double the original length."],
      },
      applyAccents: {
        description:
          "Replace ASCII characters with visually similar accented characters (e.g. a→á, e→é) to test rendering.",
      },
    },
    limitations: [
      "Does not handle right-to-left scripts.",
      "Inline codes are preserved but not expanded.",
    ],
    examples: [
      {
        title: "Basic pseudo",
        description: "Default settings",
        input: "Hello World",
        output: "[Ĥéĺĺö Ŵöŕĺð]",
      },
    ],
  },
  "qa-check": {
    displayName: "Quality Check",
    overview:
      "Runs rule-based quality assurance checks on translations. Detects whitespace mismatches, missing translations, broken inline codes, and pattern inconsistencies between source and target.",
    parameters: {
      checkLeadingWhitespace: {
        description: "Verify that leading whitespace in target matches source.",
      },
      checkInlineCodes: {
        description: "Verify all inline codes from source are preserved in target translation.",
        notes: ["Inline codes include format specifiers ({0}), HTML tags, and printf patterns."],
      },
      severityLevel: {
        description: "Default severity for issues found. Can be error, warning, or info.",
      },
    },
    processingNotes: [
      "Checks run independently — disabling one does not affect others.",
      "Results are attached as annotations to each block.",
    ],
  },
  "search-replace": {
    displayName: "Search and Replace",
    overview:
      "Performs search and replace operations on source or target text. Supports both literal string matching and Java regular expressions.",
    parameters: {
      search: { description: "The text or regex pattern to search for." },
      replace: {
        description: "The replacement text. Supports $1, $2 backreferences when regex is enabled.",
      },
      regEx: {
        description: "When enabled, the search pattern is treated as a Java regular expression.",
        notes: ["Use \\\\n for newline, \\\\t for tab in regex mode."],
      },
    },
    wikiUrl: "https://okapiframework.org/wiki/index.php/Search_and_Replace_Step",
  },
};

function getDoc(toolName: string): ToolDoc | null {
  return sampleDocs[toolName] || null;
}

const meta: Meta<typeof FlowEditor> = {
  title: "Flow Editor/FlowEditor",
  component: FlowEditor,
  tags: ["autodocs"],
  args: {
    onChange: fn(),
    onRun: fn(),
    onGetSchema: getSchema,
    onGetDoc: getDoc,
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

// ---------------------------------------------------------------------------
// I/O binding (endpoint picker) stories — a flow owns no I/O; source/sink are
// fixed endpoint terminals, not nodes (AD-026).
// ---------------------------------------------------------------------------

export const InterchangeSource: Story = {
  name: "Binding: Interchange → Files",
  args: {
    flow: {
      steps: [{ tool: "ai-translate" }, { tool: "qa-check" }],
      // Wire-format string locators: `xliff` parses to an interchange binding;
      // `file` is the default, so the sink is simply omitted.
      source: "xliff",
    },
    tools,
  },
};

export const StoreToStore: Story = {
  name: "Binding: Store → Store",
  args: {
    flow: {
      steps: [{ tool: "ai-translate" }],
      source: "store",
      sink: "store",
    },
    tools,
  },
};

export const NoSinkBinding: Story = {
  name: "Binding: Files → None (annotate in place)",
  args: {
    flow: {
      steps: [{ tool: "qa-check" }],
      // Files is the default source (omitted); sink `none` = annotate in place.
      sink: "none",
    },
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

/**
 * Flowing dots on a wrapped, multi-row pipeline. A trace is present, so DotEdge
 * streams dots along every edge. Each edge animates over the same duration, so a
 * hop to the next row (a long wrap edge) is covered as fast as a short in-row
 * hop — the cadence node-to-node is equal, only the dot's speed differs. Shown
 * in a deliberately narrow frame so the five steps wrap across rows.
 */
export const FlowingDotsMultiRow: Story = {
  name: "Flowing Dots (multi-row)",
  decorators: [
    (Story) => (
      <div style={{ width: 760, height: 660 }}>
        <Story />
      </div>
    ),
  ],
  args: {
    flow: {
      steps: [
        { tool: "tm-leverage" },
        { tool: "ai-translate" },
        { tool: "pseudo-translate" },
        { tool: "qa-check" },
        { tool: "word-count" },
      ],
    },
    tools,
    readOnly: true,
    onRun: undefined,
    // Minimal completed trace — its presence turns on the flowing-dot animation
    // (DotEdge data.flowing). One enter/exit per node keeps the overlay simple.
    traceEvents: [
      { ts: 0, type: "enter", nodeId: "tool-0", partId: "p1" },
      { ts: 100, type: "exit", nodeId: "tool-0", partId: "p1" },
      { ts: 120, type: "enter", nodeId: "tool-1", partId: "p1" },
      { ts: 220, type: "exit", nodeId: "tool-1", partId: "p1" },
      { ts: 240, type: "enter", nodeId: "tool-2", partId: "p1" },
      { ts: 340, type: "exit", nodeId: "tool-2", partId: "p1" },
      { ts: 360, type: "enter", nodeId: "tool-3", partId: "p1" },
      { ts: 460, type: "exit", nodeId: "tool-3", partId: "p1" },
      { ts: 480, type: "enter", nodeId: "tool-4", partId: "p1" },
      { ts: 580, type: "exit", nodeId: "tool-4", partId: "p1" },
    ],
  },
};

/**
 * IO-contract showcase: every node shows its typed reads → writes chips, edges
 * carry the data type flowing across them, and the legend (top-right) decodes
 * the family colors. segmentation produces a segments overlay that tm-leverage
 * optionally consumes; translate writes target; term-check / qa-check read
 * target and write findings.
 */
export const IoContractShowcase: Story = {
  name: "IO Contract Showcase",
  args: {
    flow: {
      steps: [
        { tool: "segmentation" },
        { tool: "tm-leverage" },
        { tool: "ai-translate" },
        { tool: "term-check" },
        { tool: "qa-check" },
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

/**
 * A tall parallel group (6 branches) followed by a wrap. Guards that the
 * carriage-return wrap edge clears the group instead of cutting through it —
 * row spacing grows with the tallest node so the wrap's mid-gap sweep stays
 * below the parallel (see centerAlignRows).
 */
export const ManyBranchParallel: Story = {
  name: "Many-Branch Parallel (wrap clearance)",
  args: {
    flow: {
      steps: [
        { tool: "tm-leverage", label: "TM Lookup" },
        {
          tool: "",
          parallel: [
            { tool: "qa-check", label: "Quality" },
            { tool: "brand-vocab-check", label: "Brand" },
            { tool: "entity-extract", label: "Entities" },
            { tool: "term-check", label: "Terminology" },
            { tool: "word-count", label: "Word Count" },
            { tool: "ai-translate", label: "Back-translate" },
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

// ---------------------------------------------------------------------------
// Source-transform stage stories
// ---------------------------------------------------------------------------

// Tools that declare isSourceTransform: true — in production these come from
// the backend as is_source_transform, mapped to camelCase in the API layer.
const sourceTransformAwareTools: ToolInfo[] = [
  {
    name: "redact",
    display_name: "Redact",
    description: "Replace sensitive spans with placeholders before translation",
    category: "text-processing",
    has_schema: true,
    cardinality: "monolingual",
    consumes: [{ type: "entity", side: "source", optional: true }],
    produces: [
      { type: "source", side: "source" },
      { type: "redaction.secret", side: "source" },
    ],
    tags: ["privacy", "pre-processing"],
    isSourceTransform: true,
  },
  {
    name: "source-normalise",
    display_name: "Source Normalise",
    description: "Normalise quotes, punctuation, and whitespace in source text",
    category: "text-processing",
    has_schema: true,
    cardinality: "monolingual",
    produces: [{ type: "source", side: "source" }],
    tags: ["text-processing", "pre-processing"],
    isSourceTransform: true,
  },
  {
    name: "source-simplifier",
    display_name: "Source Simplifier",
    description: "Simplify complex source sentences to aid machine translation",
    category: "text-processing",
    has_schema: false,
    cardinality: "monolingual",
    produces: [{ type: "source", side: "source" }],
    tags: ["ai-powered", "pre-processing"],
    isSourceTransform: true,
  },
  // Ordinary tools that cannot be source transforms
  ...(toolsData as ToolInfo[]).filter((t) =>
    ["ai-translate", "qa-check", "word-count", "pseudo-translate", "tm-leverage"].includes(t.name),
  ),
];

/**
 * A flow with a leading source-transform stage: redact → ai-translate → qa-check.
 * The redact node renders with the blue "pre" badge and tinted border/rail.
 */
export const WithSourceTransformStage: Story = {
  name: "Source-Transform Stage (redact → translate → qa)",
  args: {
    flow: {
      sourceTransforms: [{ tool: "redact", config: { mode: "placeholder" } }],
      steps: [{ tool: "ai-translate" }, { tool: "qa-check" }],
    },
    tools: sourceTransformAwareTools,
    onGetSchema: getSchema,
    onGetDoc: getDoc,
  },
};

/**
 * Two source-transform tools in the leading stage.
 */
export const MultipleSourceTransforms: Story = {
  name: "Source-Transform Stage (normalise + redact → translate)",
  args: {
    flow: {
      sourceTransforms: [{ tool: "source-normalise" }, { tool: "redact" }],
      steps: [{ tool: "ai-translate" }, { tool: "qa-check" }, { tool: "word-count" }],
    },
    tools: sourceTransformAwareTools,
  },
};

/**
 * Palette showing source-transform-capable tools with their "pre" badge.
 * Click any tool to open the config panel — the "Source transform" toggle
 * is enabled for redact/normalise/simplifier and disabled for translate/qa.
 */
export const SourceTransformPaletteBadges: Story = {
  name: "Source-Transform Palette Badges",
  args: {
    flow: {
      sourceTransforms: [{ tool: "redact" }],
      steps: [{ tool: "ai-translate" }, { tool: "qa-check" }],
    },
    tools: sourceTransformAwareTools,
    onGetSchema: getSchema,
    onGetDoc: getDoc,
  },
};

/**
 * Read-only view of a flow with source transforms — no palette, no config panel.
 */
export const SourceTransformReadOnly: Story = {
  name: "Source-Transform Stage (read-only)",
  args: {
    flow: {
      sourceTransforms: [{ tool: "redact" }],
      steps: [{ tool: "ai-translate" }, { tool: "qa-check" }],
    },
    tools: sourceTransformAwareTools,
    readOnly: true,
    onRun: undefined,
  },
};
