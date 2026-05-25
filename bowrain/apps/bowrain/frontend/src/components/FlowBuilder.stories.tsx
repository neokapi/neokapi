import type { Meta, StoryObj } from "@storybook/react-vite";
import { ReactFlow, Background, BackgroundVariant, Handle, Position } from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { Badge } from "@neokapi/ui";

/**
 * Storybook stories for the FlowBuilder source-transform stage.
 *
 * The full FlowBuilder depends on live Wails bindings (Backend.*), so these
 * stories render the individual node components and canvas compositions
 * directly — the review surface for the new visual treatment.
 */

const meta: Meta = {
  title: "Bowrain/FlowBuilder/SourceTransformStage",
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div
        style={{
          background: "hsl(var(--background, 224 71% 4%))",
          padding: 24,
          borderRadius: 8,
          minHeight: 300,
        }}
      >
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj;

// ---------------------------------------------------------------------------
// Shared node color palettes (mirrors FlowBuilder.tsx)
// ---------------------------------------------------------------------------

const nodeColors = {
  reader: { bg: "rgba(34, 197, 94, 0.12)", border: "#22c55e", text: "#e4e4e7", sub: "#86efac" },
  writer: { bg: "rgba(96, 165, 250, 0.12)", border: "#60a5fa", text: "#e4e4e7", sub: "#93c5fd" },
  tool: { bg: "rgba(148, 163, 184, 0.08)", border: "#64748b", text: "#e4e4e7", sub: "#94a3b8" },
  sourceTransform: {
    bg: "rgba(245, 158, 11, 0.12)",
    border: "#f59e0b",
    text: "#e4e4e7",
    sub: "#fcd34d",
  },
};

// ---------------------------------------------------------------------------
// Standalone node previews
// ---------------------------------------------------------------------------

function MainToolNodePreview({ name, description }: { name: string; description?: string }) {
  const c = nodeColors.tool;
  return (
    <div
      className="px-4 py-2.5 rounded-lg min-w-[160px] text-center text-[13px]"
      style={{ border: `2px solid ${c.border}`, background: c.bg, color: c.text }}
    >
      <div className="text-[10px] font-semibold mb-0.5" style={{ color: c.sub }}>
        TOOL
      </div>
      <div className="font-semibold">{name}</div>
      {description && (
        <div className="text-[11px] mt-0.5" style={{ color: c.sub }}>
          {description}
        </div>
      )}
    </div>
  );
}

function SourceTransformNodePreview({ name, description }: { name: string; description?: string }) {
  const c = nodeColors.sourceTransform;
  return (
    <div
      className="px-4 py-2.5 rounded-lg min-w-[160px] text-center text-[13px]"
      style={{ border: `2px solid ${c.border}`, background: c.bg, color: c.text }}
    >
      <div className="text-[10px] font-semibold mb-0.5" style={{ color: c.sub }}>
        SOURCE TRANSFORM
      </div>
      <div className="font-semibold">{name}</div>
      {description && (
        <div className="text-[11px] mt-0.5" style={{ color: c.sub }}>
          {description}
        </div>
      )}
      <div className="mt-1.5 flex justify-center">
        <span
          className="inline-flex items-center px-1.5 py-0.5 rounded text-[9px] font-bold uppercase tracking-wider"
          style={{
            background: "rgba(245, 158, 11, 0.25)",
            color: "#f59e0b",
            border: "1px solid rgba(245, 158, 11, 0.4)",
          }}
        >
          source-transform
        </span>
      </div>
    </div>
  );
}

export const MainToolNode: Story = {
  name: "Main tool node",
  render: () => (
    <div className="flex gap-4 items-start">
      <MainToolNodePreview name="ai-translate" description="Translate with AI" />
      <MainToolNodePreview name="qa-check" description="Quality check" />
    </div>
  ),
};

export const SourceTransformToolNode: Story = {
  name: "Source-transform tool node",
  render: () => (
    <div className="flex gap-4 items-start">
      <SourceTransformNodePreview name="redact" description="Redact sensitive content" />
    </div>
  ),
};

export const NodeComparison: Story = {
  name: "Node comparison — main vs source-transform",
  render: () => (
    <div className="space-y-6">
      <div>
        <p className="text-xs text-muted-foreground mb-2">
          Main stage tool — grey border, no badge:
        </p>
        <MainToolNodePreview name="ai-translate" description="Translate with AI/LLM" />
      </div>
      <div>
        <p className="text-xs text-muted-foreground mb-2">
          Source-transform stage tool — amber accent, badge, settles the model first:
        </p>
        <SourceTransformNodePreview name="redact" description="Redact sensitive content" />
      </div>
    </div>
  ),
};

// ---------------------------------------------------------------------------
// Full canvas: secure-translate flow (redact → ai-translate)
// ---------------------------------------------------------------------------

function ReaderNodeC({ data }: { data: Record<string, unknown> }) {
  const c = nodeColors.reader;
  return (
    <div
      className="px-4 py-2.5 rounded-lg min-w-[140px] text-center text-[13px]"
      style={{ border: `2px solid ${c.border}`, background: c.bg, color: c.text }}
    >
      <div className="text-[10px] font-semibold mb-0.5" style={{ color: c.border }}>
        INPUT
      </div>
      <div className="font-semibold">{(data.label as string) || "Reader"}</div>
      <Handle type="source" position={Position.Right} style={{ background: c.border }} />
    </div>
  );
}

function WriterNodeC({ data }: { data: Record<string, unknown> }) {
  const c = nodeColors.writer;
  return (
    <div
      className="px-4 py-2.5 rounded-lg min-w-[140px] text-center text-[13px]"
      style={{ border: `2px solid ${c.border}`, background: c.bg, color: c.text }}
    >
      <Handle type="target" position={Position.Left} style={{ background: c.border }} />
      <div className="text-[10px] font-semibold mb-0.5" style={{ color: c.border }}>
        OUTPUT
      </div>
      <div className="font-semibold">{(data.label as string) || "Writer"}</div>
    </div>
  );
}

function ToolNodeC({ data }: { data: Record<string, unknown> }) {
  const isSourceTransform = data.stage === "source-transform";
  const c = isSourceTransform ? nodeColors.sourceTransform : nodeColors.tool;
  return (
    <div
      className="px-4 py-2.5 rounded-lg min-w-[140px] text-center text-[13px]"
      style={{ border: `2px solid ${c.border}`, background: c.bg, color: c.text }}
    >
      <Handle type="target" position={Position.Left} style={{ background: c.border }} />
      <div className="text-[10px] font-semibold mb-0.5" style={{ color: c.sub }}>
        {isSourceTransform ? "SOURCE TRANSFORM" : "TOOL"}
      </div>
      <div className="font-semibold">{(data.label as string) || (data.toolName as string)}</div>
      {isSourceTransform && (
        <div className="mt-1.5 flex justify-center">
          <span
            className="inline-flex items-center px-1.5 py-0.5 rounded text-[9px] font-bold uppercase tracking-wider"
            style={{
              background: "rgba(245, 158, 11, 0.25)",
              color: "#f59e0b",
              border: "1px solid rgba(245, 158, 11, 0.4)",
            }}
          >
            source-transform
          </span>
        </div>
      )}
      <Handle type="source" position={Position.Right} style={{ background: c.border }} />
    </div>
  );
}

const storyNodeTypes = {
  reader: ReaderNodeC as React.ComponentType<{ data: Record<string, unknown> }>,
  writer: WriterNodeC as React.ComponentType<{ data: Record<string, unknown> }>,
  tool: ToolNodeC as React.ComponentType<{ data: Record<string, unknown> }>,
};

const secureTranslateNodes = [
  {
    id: "reader",
    type: "reader",
    position: { x: 0, y: 80 },
    data: { label: "Input" },
  },
  {
    id: "redact",
    type: "tool",
    position: { x: 220, y: 60 },
    data: { label: "Redact", toolName: "redact", stage: "source-transform" },
  },
  {
    id: "ai-translate",
    type: "tool",
    position: { x: 460, y: 60 },
    data: { label: "AI Translate", toolName: "ai-translate", stage: "" },
  },
  {
    id: "unredact",
    type: "tool",
    position: { x: 700, y: 60 },
    data: { label: "Unredact", toolName: "unredact", stage: "" },
  },
  {
    id: "writer",
    type: "writer",
    position: { x: 940, y: 80 },
    data: { label: "Output" },
  },
];

const secureTranslateEdges = [
  {
    id: "e1",
    source: "reader",
    target: "redact",
    animated: true,
    style: { stroke: "#6366f1", strokeWidth: 2 },
  },
  {
    id: "e2",
    source: "redact",
    target: "ai-translate",
    animated: true,
    style: { stroke: "#6366f1", strokeWidth: 2 },
  },
  {
    id: "e3",
    source: "ai-translate",
    target: "unredact",
    animated: true,
    style: { stroke: "#6366f1", strokeWidth: 2 },
  },
  {
    id: "e4",
    source: "unredact",
    target: "writer",
    animated: true,
    style: { stroke: "#6366f1", strokeWidth: 2 },
  },
];

export const SecureTranslateFlow: Story = {
  name: "Secure Translate flow (redact → ai-translate → unredact)",
  render: () => (
    <div className="space-y-3">
      <div className="flex items-center gap-2">
        <span className="text-sm font-semibold text-foreground">Secure Translate</span>
        <Badge variant="secondary">built-in</Badge>
      </div>
      <p className="text-xs text-muted-foreground">
        The <strong style={{ color: "#f59e0b" }}>Redact</strong> node sits in the source-transform
        stage (amber accent + badge). It rewrites the source before the main tools see it —
        downstream translates redacted content, then Unredact restores originals locally.
      </p>
      <div
        style={{
          height: 220,
          border: "1px solid hsl(var(--border, 240 3.7% 20%))",
          borderRadius: 8,
          overflow: "hidden",
        }}
      >
        <ReactFlow
          nodes={secureTranslateNodes}
          edges={secureTranslateEdges}
          nodeTypes={storyNodeTypes as unknown as import("@xyflow/react").NodeTypes}
          fitView
          proOptions={{ hideAttribution: true }}
          nodesDraggable={false}
          nodesConnectable={false}
          elementsSelectable={false}
          className="bg-background"
        >
          <Background variant={BackgroundVariant.Dots} gap={16} size={1} color="#3e4047" />
        </ReactFlow>
      </div>
    </div>
  ),
};

export const SourceTransformToggleDisabled: Story = {
  name: "Source-transform toggle — disabled (tool not capable)",
  render: () => (
    <div className="space-y-4 max-w-72">
      <p className="text-xs text-muted-foreground">
        When the selected tool does not have <code>is_source_transform: true</code>, the toggle is
        disabled with an explanatory tooltip. The node stays in the main chain.
      </p>
      <div className="border border-border rounded-lg overflow-hidden">
        <div className="px-4 py-3 border-b border-border">
          <div className="flex items-center justify-between gap-2">
            <div className="flex flex-col gap-0.5">
              <span className="text-sm font-medium">Source Transform</span>
              <span className="text-[11px] text-muted-foreground">
                Runs before main tools to settle the source model
              </span>
            </div>
            {/* Disabled switch mock */}
            <span
              className="inline-flex items-center w-9 h-5 rounded-full"
              style={{
                background: "hsl(var(--muted, 240 3.7% 26%))",
                opacity: 0.5,
                cursor: "not-allowed",
              }}
            />
          </div>
          <p className="mt-1.5 text-[11px] text-muted-foreground italic">
            Tooltip: "This tool does not support the source-transform stage."
          </p>
        </div>
        <div className="p-4 text-sm text-muted-foreground">
          No configurable parameters for <span className="font-mono">ai-translate</span>
        </div>
      </div>
    </div>
  ),
};

export const SourceTransformToggleEnabled: Story = {
  name: "Source-transform toggle — enabled and active (redact tool)",
  render: () => (
    <div className="space-y-4 max-w-72">
      <p className="text-xs text-muted-foreground">
        When the tool is capable (<code>is_source_transform: true</code>) and the toggle is ON, the
        node shifts to amber styling and the amber callout appears.
      </p>
      <div className="border border-border rounded-lg overflow-hidden">
        <div className="px-4 py-3 border-b border-border">
          <div className="flex items-center justify-between gap-2">
            <div className="flex flex-col gap-0.5">
              <span className="text-sm font-medium">Source Transform</span>
              <span className="text-[11px] text-muted-foreground">
                Runs before main tools to settle the source model
              </span>
            </div>
            {/* Enabled/checked switch mock */}
            <span
              className="inline-flex items-center w-9 h-5 rounded-full"
              style={{ background: "#f59e0b" }}
            >
              <span
                className="inline-block w-4 h-4 rounded-full ml-auto mr-0.5"
                style={{ background: "white" }}
              />
            </span>
          </div>
          {/* Amber callout */}
          <div
            className="mt-2 px-2 py-1.5 rounded text-[11px]"
            style={{
              background: "rgba(245, 158, 11, 0.1)",
              color: "#f59e0b",
              border: "1px solid rgba(245, 158, 11, 0.3)",
            }}
          >
            Runs ahead of main tools — downstream sees a settled source.
          </div>
        </div>
        <div className="p-4 text-sm text-muted-foreground">
          No configurable parameters for <span className="font-mono">redact</span>
        </div>
      </div>
    </div>
  ),
};
