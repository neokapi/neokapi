import type { Meta, StoryObj } from "@storybook/react-vite";
import { ReactFlow, ReactFlowProvider } from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { ConceptNode } from "../../components/terms/ConceptNode";
import type { ConceptNodeData } from "../../components/terms/ConceptNode";

const NODE_TYPES = { concept: ConceptNode };

/**
 * Helper that renders a single ConceptNode inside a minimal React Flow canvas
 * so the Handle components mount correctly.
 */
function NodePreview({ data, width = 300 }: { data: ConceptNodeData; width?: number }) {
  const nodes = [
    {
      id: "preview",
      type: "concept" as const,
      position: { x: 40, y: 30 },
      data: data as unknown as Record<string, unknown>,
    },
  ];

  return (
    <ReactFlowProvider>
      <div style={{ width, height: 200 }}>
        <ReactFlow
          nodes={nodes}
          edges={[]}
          nodeTypes={NODE_TYPES}
          fitView
          fitViewOptions={{ padding: 0.5 }}
          nodesDraggable={false}
          nodesConnectable={false}
          panOnDrag={false}
          zoomOnScroll={false}
          zoomOnDoubleClick={false}
          proOptions={{ hideAttribution: true }}
        />
      </div>
    </ReactFlowProvider>
  );
}

const meta: Meta = {
  title: "Terms/ConceptNode",
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "A custom React Flow node that renders a concept as a compact card. " +
          "Shows domain tag, preferred term, definition preview, and stats badges.",
      },
    },
  },
};

export default meta;
type Story = StoryObj;

/** Fully populated concept node with all badges visible */
export const Default: Story = {
  render: () => (
    <NodePreview
      data={{
        conceptId: "c-auth",
        preferredTerm: "Authentication",
        domain: "Security",
        definition:
          "The process of verifying a user's identity before granting access to the system.",
        localeCount: 3,
        termCount: 5,
        childCount: 1,
        parentCount: 0,
        isSelected: false,
      }}
    />
  ),
};

/** Node in selected state — shows ring highlight */
export const Selected: Story = {
  render: () => (
    <NodePreview
      data={{
        conceptId: "c-auth",
        preferredTerm: "Authentication",
        domain: "Security",
        definition:
          "The process of verifying a user's identity before granting access to the system.",
        localeCount: 3,
        termCount: 5,
        childCount: 1,
        parentCount: 0,
        isSelected: true,
      }}
    />
  ),
};

/** Minimal node — single language, single term, no links, no domain */
export const Minimal: Story = {
  render: () => (
    <NodePreview
      data={{
        conceptId: "c-min",
        preferredTerm: "Button",
        domain: "",
        definition: "",
        localeCount: 1,
        termCount: 1,
        childCount: 0,
        parentCount: 0,
        isSelected: false,
      }}
    />
  ),
};

/** Node with a long term and definition to test truncation */
export const LongContent: Story = {
  render: () => (
    <NodePreview
      data={{
        conceptId: "c-long",
        preferredTerm: "Internationalization and Localization Framework",
        domain: "Engineering",
        definition:
          "A comprehensive software framework that enables products to be adapted for different languages, " +
          "regions, and cultures, including text direction, date formats, number formatting, and currency display.",
        localeCount: 12,
        termCount: 24,
        childCount: 5,
        parentCount: 2,
        isSelected: false,
      }}
    />
  ),
};

/** Side-by-side comparison of selected vs unselected */
export const SelectionComparison: Story = {
  render: () => (
    <div style={{ display: "flex", gap: 24 }}>
      <div>
        <div style={{ fontSize: 11, color: "#888", marginBottom: 4 }}>Unselected</div>
        <NodePreview
          data={{
            conceptId: "c-1",
            preferredTerm: "Workspace",
            domain: "Platform",
            definition: "A shared organizational context.",
            localeCount: 3,
            termCount: 4,
            childCount: 1,
            parentCount: 0,
            isSelected: false,
          }}
        />
      </div>
      <div>
        <div style={{ fontSize: 11, color: "#888", marginBottom: 4 }}>Selected</div>
        <NodePreview
          data={{
            conceptId: "c-2",
            preferredTerm: "Workspace",
            domain: "Platform",
            definition: "A shared organizational context.",
            localeCount: 3,
            termCount: 4,
            childCount: 1,
            parentCount: 0,
            isSelected: true,
          }}
        />
      </div>
    </div>
  ),
};
