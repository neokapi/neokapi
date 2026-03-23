import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ReactFlowProvider } from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { ConceptGraph } from "../../components/terms/ConceptGraph";
import type { ConceptInfo } from "../../types/api";
import { concepts, hierarchy, graphEdges } from "./concept-fixtures";

/**
 * Wrapper that provides ReactFlowProvider and manages selection state,
 * mirroring how ConceptExplorer uses ConceptGraph.
 */
function InteractiveGraph({
  initialSelectedId = null,
  showConcepts = concepts,
}: {
  initialSelectedId?: string | null;
  showConcepts?: ConceptInfo[];
}) {
  const [selectedId, setSelectedId] = useState<string | null>(initialSelectedId);

  return (
    <ReactFlowProvider>
      <ConceptGraph
        concepts={showConcepts}
        hierarchy={hierarchy}
        graphEdges={graphEdges}
        selectedId={selectedId}
        onSelectConcept={(c) => setSelectedId(c.id)}
        onNavigateConcept={fn()}
      />
    </ReactFlowProvider>
  );
}

const meta: Meta<typeof ConceptGraph> = {
  title: "Terms/ConceptGraph",
  component: ConceptGraph,
  tags: ["autodocs"],
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "Interactive React Flow canvas that visualizes concepts as card nodes connected by " +
          "SKOS-aligned, color-coded edges. Includes minimap, zoom controls, and a relationship legend.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof ConceptGraph>;

/** Full graph with all 9 concepts and 7 relationship edges. Click a node to select it. */
export const Default: Story = {
  render: () => (
    <div style={{ width: "100vw", height: "100vh" }}>
      <InteractiveGraph />
    </div>
  ),
};

/** Graph with Authentication pre-selected — shows the highlight ring and centered layout. */
export const WithSelection: Story = {
  render: () => (
    <div style={{ width: "100vw", height: "100vh" }}>
      <InteractiveGraph initialSelectedId="c-auth" />
    </div>
  ),
};

/** Small graph — only two connected concepts. Tests minimal edge rendering. */
export const TwoNodes: Story = {
  render: () => {
    const pair = concepts.filter((c) => c.id === "c-auth" || c.id === "c-authz");
    return (
      <div style={{ width: "100vw", height: "100vh" }}>
        <InteractiveGraph showConcepts={pair} />
      </div>
    );
  },
};

/** Single node — no edges, no legend. */
export const SingleNode: Story = {
  render: () => {
    const single = concepts.filter((c) => c.id === "c-brand-voice");
    return (
      <div style={{ width: "100vw", height: "100vh" }}>
        <ReactFlowProvider>
          <ConceptGraph
            concepts={single}
            hierarchy={hierarchy}
            graphEdges={[]}
            selectedId={null}
            onSelectConcept={fn()}
          />
        </ReactFlowProvider>
      </div>
    );
  },
};
