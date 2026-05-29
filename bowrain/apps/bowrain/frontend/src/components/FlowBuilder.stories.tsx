import type { Meta, StoryObj } from "@storybook/react-vite";
import { FlowEditor, type FlowSpec, type ToolInfo } from "@neokapi/flow-editor";

/**
 * Storybook stories for the Bowrain flow builder.
 *
 * The Bowrain flow builder renders the shared `@neokapi/flow-editor`
 * <FlowEditor> — the same canonical component kapi-desktop uses — bridged from
 * the backend's node/edge flow definitions via the defToSpec/specToDef adapter.
 * These stories exercise that component directly with Bowrain-flavored data;
 * the source-transform stage treatment (palette badge, amber node styling, and
 * the "Settles the model before main tools run" toggle copy) all come from the
 * shared editor, so there is a single source of truth for the visual surface.
 *
 * The full FlowBuilder additionally needs live Wails bindings (the flow list +
 * save/delete chrome), which are covered by the Playwright e2e suite.
 */

const tools: ToolInfo[] = [
  {
    name: "redact",
    display_name: "Redact",
    description: "Replace sensitive spans with placeholders before translation.",
    category: "transform",
    isSourceTransform: true,
  },
  {
    name: "unredact",
    display_name: "Unredact",
    description: "Restore the original spans locally after translation.",
    category: "transform",
  },
  {
    name: "ai-translate",
    display_name: "AI Translate",
    description: "Translate content using an AI/LLM provider.",
    category: "translate",
  },
  {
    name: "ai-qa",
    display_name: "AI QA",
    description: "Quality-check translations using AI.",
    category: "validate",
  },
  {
    name: "pseudo-translate",
    display_name: "Pseudo Translate",
    description: "Generate pseudo-translations for layout testing.",
    category: "transform",
  },
];

const noop = () => {};

const meta: Meta<typeof FlowEditor> = {
  title: "Bowrain/FlowBuilder",
  component: FlowEditor,
  tags: ["autodocs"],
  args: {
    tools,
    onChange: noop,
  },
  decorators: [
    (Story) => (
      <div style={{ height: 480, border: "1px solid var(--border)", borderRadius: 8 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof FlowEditor>;

const secureTranslate: FlowSpec = {
  description: "Redact, AI-translate, then restore originals locally.",
  sourceTransforms: [{ tool: "redact" }],
  steps: [{ tool: "ai-translate" }, { tool: "unredact" }],
};

const aiTranslateQa: FlowSpec = {
  description: "Translate with AI then run a QA check.",
  steps: [{ tool: "ai-translate" }, { tool: "ai-qa" }],
};

export const SecureTranslate: Story = {
  name: "Secure Translate (source-transform stage)",
  args: { flow: secureTranslate },
};

export const SecureTranslateReadOnly: Story = {
  name: "Secure Translate — read-only (built-in)",
  args: { flow: secureTranslate, readOnly: true },
};

export const AiTranslateQa: Story = {
  name: "AI Translate + QA",
  args: { flow: aiTranslateQa },
};
