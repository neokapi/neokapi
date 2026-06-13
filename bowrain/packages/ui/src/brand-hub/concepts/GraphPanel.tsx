// The interactive concept knowledge graph (AD-021), rendered with React Flow.
// Nodes are concept cards accented by dominant term status; edges are typed,
// relation-styled connectors with arrowheads and labels. Layout is the
// deterministic levelled layout from graph-layout.ts, so the same graph always
// settles the same way and React Flow only has to draw it. Self-contained: the
// component owns its ReactFlowProvider, so callers just hand it a graph.
//
// The props are intentionally minimal (graph + focus + selection) — the smart
// controls (search, as-of/market filters, legend, side panel) live in
// ConceptGraphView, which composes this canvas.
import { useEffect, useMemo } from "react";
import {
  ReactFlow,
  ReactFlowProvider,
  Background,
  BackgroundVariant,
  Controls,
  MiniMap,
  MarkerType,
  useNodesState,
  useEdgesState,
  useReactFlow,
  type NodeTypes,
  type EdgeTypes,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { cn } from "@neokapi/ui-primitives";
import type { GraphViz } from "../../types/brand-graph";
import { relationLabel } from "../shell/atoms";
import { layoutConcepts } from "./graph-layout";
import { relationEdgeStyle, orientEdge, neighbourhood, statusColorVar } from "./graph-style";
import {
  ConceptNode,
  CONCEPT_NODE_WIDTH,
  CONCEPT_NODE_HEIGHT,
  type ConceptFlowNode,
  type ConceptNodeData,
} from "./ConceptNode";
import { ConceptEdge, type ConceptFlowEdge } from "./ConceptEdge";

const nodeTypes: NodeTypes = { concept: ConceptNode };
const edgeTypes: EdgeTypes = { concept: ConceptEdge };

export interface GraphPanelProps {
  graph: GraphViz;
  /** Highlight a focused concept (its neighbourhood stays bright; the rest fades). */
  focusId?: string;
  onSelectNode?: (id: string) => void;
  className?: string;
}

export function GraphPanel(props: GraphPanelProps) {
  return (
    <ReactFlowProvider>
      <GraphCanvas {...props} />
    </ReactFlowProvider>
  );
}

function GraphCanvas({ graph, focusId, onSelectNode, className }: GraphPanelProps) {
  const [nodes, setNodes, onNodesChange] = useNodesState<ConceptFlowNode>([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState<ConceptFlowEdge>([]);
  const rf = useReactFlow();

  const layout = useMemo(() => layoutConcepts(graph), [graph]);
  const nbh = useMemo(() => neighbourhood(graph, focusId), [graph, focusId]);
  const focusing = !!focusId && nbh.nodeIds.size > 0;

  // Build (and rebuild on graph/focus change) the React Flow node set. Positions
  // come from the deterministic layout; focus only flips the dim/highlight flags.
  useEffect(() => {
    setNodes(
      graph.nodes.map((n) => {
        const pos = layout.nodes[n.id] ?? { x: 0, y: 0 };
        return {
          id: n.id,
          type: "concept",
          position: { x: pos.x, y: pos.y },
          style: { width: CONCEPT_NODE_WIDTH, height: CONCEPT_NODE_HEIGHT },
          data: {
            label: n.label || n.id,
            domain: n.domain,
            status: n.status,
            term_count: n.term_count,
            focused: focusing && n.id === focusId,
            dimmed: focusing && !nbh.nodeIds.has(n.id),
          },
        };
      }),
    );
  }, [graph, layout, nbh, focusId, focusing, setNodes]);

  useEffect(() => {
    setEdges(
      graph.edges.map((e) => {
        const { source, target } = orientEdge(e);
        const style = relationEdgeStyle(e.type);
        const dimmed = focusing && !nbh.edgeIds.has(e.id);
        return {
          id: e.id,
          type: "concept",
          source,
          target,
          animated: style.animated && !dimmed,
          markerEnd: style.directed
            ? { type: MarkerType.ArrowClosed, color: style.color, width: 16, height: 16 }
            : undefined,
          data: { relation: e.type, label: relationLabel(e.type), dimmed },
        };
      }),
    );
  }, [graph, nbh, focusing, setEdges]);

  // Frame the whole graph when it loads/changes (no focus), or zoom to the
  // focused concept when one is chosen.
  useEffect(() => {
    if (nodes.length === 0) return;
    const t = window.setTimeout(() => {
      if (focusing && focusId) {
        void rf.fitView({ nodes: [{ id: focusId }], duration: 450, maxZoom: 1.3, padding: 0.5 });
      } else {
        void rf.fitView({ padding: 0.18, maxZoom: 1.15, duration: 300 });
      }
    }, 60);
    return () => window.clearTimeout(t);
  }, [nodes.length, focusId, focusing, rf]);

  return (
    <div className={cn("h-full w-full", className)}>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        nodeTypes={nodeTypes}
        edgeTypes={edgeTypes}
        onNodeClick={(_, node) => onSelectNode?.(node.id)}
        nodesConnectable={false}
        nodesDraggable
        elementsSelectable
        minZoom={0.2}
        maxZoom={2}
        fitView
        fitViewOptions={{ padding: 0.18, maxZoom: 1.15 }}
        proOptions={{ hideAttribution: true }}
        className="bg-muted/20"
        aria-label="Concept relationship graph"
      >
        <Background variant={BackgroundVariant.Dots} gap={22} size={1} className="text-border" />
        <Controls showInteractive={false} className="!shadow-sm" />
        {graph.nodes.length > 4 && (
          <MiniMap
            pannable
            zoomable
            ariaLabel="Graph minimap"
            className="!bg-card !rounded-md !border"
            maskColor="var(--color-muted)"
            nodeColor={(n) => statusColorVar((n.data as ConceptNodeData).status)}
            nodeStrokeWidth={0}
          />
        )}
      </ReactFlow>
    </div>
  );
}
