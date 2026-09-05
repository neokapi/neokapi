import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import type { FlowTrace } from "@neokapi/flow-editor";
import { FlowPage } from "../components/FlowPage";
import type { FlowSpec, RunTraces } from "../types/api";

// A recorded run of the flow below, in the shape the backend retains it:
// reader and writer nodes bracket one tool node per step, every part is
// snapshotted after each step, and events carry the timing.
const recordedFlow: FlowSpec = {
  steps: [{ tool: "redact", config: { detectors: ["entities"] } }, { tool: "translate" }],
};

const recordedTrace: FlowTrace = {
  name: "secure-translate",
  description: "secure-translate flow on contact-page.md",
  nodes: [
    { id: "reader", type: "reader", name: "markdown", label: "markdown reader" },
    { id: "tool-0", type: "tool", name: "redact", label: "redact" },
    { id: "tool-1", type: "tool", name: "translate", label: "translate" },
    { id: "writer", type: "writer", name: "markdown", label: "markdown writer" },
  ],
  events: [
    { ts: 40, type: "exit", nodeId: "reader", partId: "b1" },
    { ts: 120, type: "enter", nodeId: "tool-0", partId: "b1" },
    { ts: 480, type: "exit", nodeId: "tool-0", partId: "b1" },
    { ts: 510, type: "enter", nodeId: "tool-1", partId: "b1" },
    { ts: 2200, type: "exit", nodeId: "tool-1", partId: "b1" },
    { ts: 2210, type: "enter", nodeId: "writer", partId: "b1" },
    { ts: 2230, type: "exit", nodeId: "writer", partId: "b1" },
    { ts: 2250, type: "exit", nodeId: "reader", partId: "b2" },
    { ts: 2300, type: "enter", nodeId: "tool-0", partId: "b2" },
    { ts: 2350, type: "exit", nodeId: "tool-0", partId: "b2" },
    { ts: 2400, type: "enter", nodeId: "tool-1", partId: "b2" },
    { ts: 3100, type: "exit", nodeId: "tool-1", partId: "b2" },
    { ts: 3110, type: "enter", nodeId: "writer", partId: "b2" },
    { ts: 3140, type: "exit", nodeId: "writer", partId: "b2" },
  ],
  parts: {
    b1: {
      initial: {
        id: "b1",
        type: "Block",
        summary: "Contact Jane Doe at Acme Corp",
        sourceText: "Contact Jane Doe at Acme Corp",
        detail: {
          overlays: [
            {
              type: "entity",
              side: "source",
              spans: [
                { start: 8, end: 16, text: "Jane Doe", note: "entity:person" },
                { start: 20, end: 29, text: "Acme Corp", note: "entity:organization" },
              ],
            },
          ],
        },
      },
      afterNode: {
        "tool-0": {
          id: "b1",
          type: "Block",
          summary: "Contact Jane Doe at Acme Corp",
          sourceText: "Contact [REDACTED:Person] at [REDACTED:Org]",
          detail: { annotations: [{ key: "redaction.secret", summary: "2 vaulted originals" }] },
        },
        "tool-1": {
          id: "b1",
          type: "Block",
          summary: "Contact Jane Doe at Acme Corp",
          sourceText: "Contact [REDACTED:Person] at [REDACTED:Org]",
          targetText: "Contactez [REDACTED:Person] chez [REDACTED:Org]",
          detail: { annotations: [{ key: "redaction.secret", summary: "2 vaulted originals" }] },
        },
      },
    },
    b2: {
      initial: {
        id: "b2",
        type: "Block",
        summary: "Thanks for reaching out!",
        sourceText: "Thanks for reaching out!",
      },
      afterNode: {
        "tool-0": {
          id: "b2",
          type: "Block",
          summary: "Thanks for reaching out!",
          sourceText: "Thanks for reaching out!",
        },
        "tool-1": {
          id: "b2",
          type: "Block",
          summary: "Thanks for reaching out!",
          sourceText: "Thanks for reaching out!",
          targetText: "Merci de nous avoir contactés !",
        },
      },
    },
  },
  durationUs: 3200,
};

const recordedRun: RunTraces = {
  flow_name: "secure-translate",
  steps: recordedFlow.steps,
  files: [
    { file_path: "/projects/site/src/contact-page.md", locale: "fr-FR" },
    { file_path: "/projects/site/src/pricing.md", locale: "fr-FR", truncated: true },
  ],
  max_parts: 500,
};

const meta: Meta<typeof FlowPage> = {
  title: "Pages/FlowPage",
  component: FlowPage,
  tags: ["autodocs"],
  args: {
    onChange: fn(),
    onRun: fn(),
  },
  parameters: {
    layout: "fullscreen",
  },
  decorators: [
    (Story) => (
      <div style={{ height: 600 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof FlowPage>;

export const WithFlows: Story = {
  args: {
    flowName: "translate",
    flow: {
      steps: [{ tool: "translate", config: { provider: "anthropic" } }],
    },
  },
};

export const Empty: Story = {
  args: {
    flowName: "new-flow",
    flow: {
      steps: [],
    },
  },
};

/**
 * The flow after a run: the Run view replays the retained trace on the
 * canvas, the view bar names the file (a select when the run covered
 * several), and a trace the recording budget cut short says so.
 */
export const WithRun: Story = {
  name: "With a recorded run",
  args: {
    flowName: "secure-translate",
    flow: recordedFlow,
    preloadedRun: {
      run: recordedRun,
      traces: {
        "/projects/site/src/contact-page.md": recordedTrace,
        "/projects/site/src/pricing.md": recordedTrace,
      },
    },
  },
};

/**
 * The same run after the flow was edited: the trace no longer describes the
 * steps shown, so the Run view is withheld until the edit is undone.
 */
export const EditedSinceRun: Story = {
  name: "Edited since the run (no Run view)",
  args: {
    flowName: "secure-translate",
    flow: { steps: [{ tool: "translate" }] },
    preloadedRun: {
      run: recordedRun,
      traces: { "/projects/site/src/contact-page.md": recordedTrace },
    },
  },
};

export const ParallelFlow: Story = {
  name: "Parallel group (switch to Diagram)",
  args: {
    flowName: "translate-and-check",
    flow: {
      description: "Translate, then check and count in parallel.",
      steps: [
        { tool: "translate", config: { provider: "anthropic" } },
        { tool: "", parallel: [{ tool: "qa" }, { tool: "word-count" }] },
      ],
    },
  },
};
