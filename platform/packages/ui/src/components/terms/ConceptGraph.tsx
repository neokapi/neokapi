import { useCallback, useMemo, useEffect, useRef } from "react";
import {
  ReactFlow,
  Controls,
  MiniMap,
  Background,
  BackgroundVariant,
  useNodesState,
  useEdgesState,
  MarkerType,
  Panel,
} from "@xyflow/react";
import type { Node, Edge as FlowEdge, NodeMouseHandler } from "@xyflow/react";
import "@xyflow/react/dist/style.css";

import { ConceptNode } from "./ConceptNode";
import type { ConceptNodeData } from "./ConceptNode";
import type { ConceptInfo, ConceptHierarchyNode, GraphEdge } from "../../types/api";
import { GRAPH_LABEL_DISPLAY } from "../../types/api";

/** Edge-label coloring by relationship type */
const EDGE_COLORS: Record<string, string> = {
  BROADER: "hsl(220 70% 55%)",
  NARROWER: "hsl(220 70% 55%)",
  RELATED: "hsl(160 60% 45%)",
  PART_OF: "hsl(280 60% 55%)",
  HAS_PART: "hsl(280 60% 55%)",
  USE_INSTEAD: "hsl(30 90% 50%)",
  REPLACED_BY: "hsl(30 90% 50%)",
  EXACT_MATCH: "hsl(340 70% 55%)",
  CLOSE_MATCH: "hsl(340 50% 65%)",
  FORBIDDEN: "hsl(0 70% 50%)",
  PREFERRED: "hsl(140 60% 45%)",
  COMPETITOR: "hsl(0 60% 60%)",
};

const NODE_TYPES = { concept: ConceptNode };

interface ConceptGraphProps {
  /** All concepts to display */
  concepts: ConceptInfo[];
  /** Hierarchy data from /graph/concepts endpoint */
  hierarchy: ConceptHierarchyNode[];
  /** Graph edges for connected concepts */
  graphEdges: GraphEdge[];
  /** Currently selected concept ID */
  selectedId: string | null;
  /** Called when a concept node is clicked */
  onSelectConcept: (concept: ConceptInfo) => void;
  /** Called when the user wants to navigate deeper into a concept */
  onNavigateConcept?: (conceptId: string) => void;
}

/** Simple force-directed-ish layout: arrange nodes in concentric rings around the center */
function layoutNodes(
  concepts: ConceptInfo[],
  hierarchy: ConceptHierarchyNode[],
  selectedId: string | null,
): Node[] {
  const hierMap = new Map(hierarchy.map((h) => [h.id, h]));
  const centerX = 0;
  const centerY = 0;

  // Put selected node in center, arrange others around it
  const sorted = [...concepts].sort((a, b) => {
    if (a.id === selectedId) return -1;
    if (b.id === selectedId) return 1;
    const ha = hierMap.get(a.id);
    const hb = hierMap.get(b.id);
    const scoreA = (ha?.children ?? 0) + (ha?.parents ?? 0);
    const scoreB = (hb?.children ?? 0) + (hb?.parents ?? 0);
    return scoreB - scoreA;
  });

  return sorted.map((concept, i) => {
    const hier = hierMap.get(concept.id);
    const locales = new Set(concept.terms.map((t) => t.locale));
    const preferred = concept.terms.find((t) => t.status === "preferred") ?? concept.terms[0];

    let x: number, y: number;
    if (i === 0 && concept.id === selectedId) {
      x = centerX;
      y = centerY;
    } else {
      // Arrange in rings
      const ring = Math.floor((i - (selectedId ? 1 : 0)) / 8);
      const posInRing = (i - (selectedId ? 1 : 0)) % 8;
      const radius = 300 + ring * 280;
      const angle = (posInRing / 8) * Math.PI * 2 - Math.PI / 2;
      x = centerX + Math.cos(angle) * radius;
      y = centerY + Math.sin(angle) * radius;
    }

    return {
      id: concept.id,
      type: "concept",
      position: { x, y },
      data: {
        conceptId: concept.id,
        preferredTerm: preferred?.text ?? concept.id,
        domain: concept.domain,
        definition: concept.definition,
        localeCount: locales.size,
        termCount: concept.terms.length,
        childCount: hier?.children ?? 0,
        parentCount: hier?.parents ?? 0,
        isSelected: concept.id === selectedId,
        source: concept.project_id ? "project" : "workspace",
      } satisfies ConceptNodeData,
    };
  });
}

function buildEdges(graphEdges: GraphEdge[], conceptIds: Set<string>): FlowEdge[] {
  return graphEdges
    .filter((e) => conceptIds.has(e.source) && conceptIds.has(e.target))
    .map((e) => ({
      id: e.id,
      source: e.source,
      target: e.target,
      label: GRAPH_LABEL_DISPLAY[e.label] ?? e.label,
      type: "smoothstep",
      animated: e.label === "USE_INSTEAD" || e.label === "REPLACED_BY",
      style: { stroke: EDGE_COLORS[e.label] ?? "hsl(var(--muted-foreground))" },
      labelStyle: {
        fontSize: 10,
        fill: EDGE_COLORS[e.label] ?? "hsl(var(--muted-foreground))",
        fontWeight: 500,
      },
      markerEnd: {
        type: MarkerType.ArrowClosed,
        width: 16,
        height: 16,
        color: EDGE_COLORS[e.label] ?? "hsl(var(--muted-foreground))",
      },
    }));
}

export function ConceptGraph({
  concepts,
  hierarchy,
  graphEdges,
  selectedId,
  onSelectConcept,
}: ConceptGraphProps) {
  const conceptIds = useMemo(() => new Set(concepts.map((c) => c.id)), [concepts]);

  const initialNodes = useMemo(
    () => layoutNodes(concepts, hierarchy, selectedId),
    [concepts, hierarchy, selectedId],
  );
  const initialEdges = useMemo(() => buildEdges(graphEdges, conceptIds), [graphEdges, conceptIds]);

  const [nodes, setNodes, onNodesChange] = useNodesState(initialNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges);

  // Sync when concepts or edges change
  const prevConceptsRef = useRef(concepts);
  const prevEdgesRef = useRef(graphEdges);
  useEffect(() => {
    if (prevConceptsRef.current !== concepts || prevEdgesRef.current !== graphEdges) {
      prevConceptsRef.current = concepts;
      prevEdgesRef.current = graphEdges;
      setNodes(layoutNodes(concepts, hierarchy, selectedId));
      setEdges(buildEdges(graphEdges, conceptIds));
    }
  }, [concepts, hierarchy, graphEdges, selectedId, conceptIds, setNodes, setEdges]);

  // Update selection highlight without re-layouting
  useEffect(() => {
    setNodes((nds) =>
      nds.map((n) => ({
        ...n,
        data: { ...n.data, isSelected: n.id === selectedId },
      })),
    );
  }, [selectedId, setNodes]);

  const conceptMap = useMemo(() => new Map(concepts.map((c) => [c.id, c])), [concepts]);

  const onNodeClick: NodeMouseHandler = useCallback(
    (_event, node) => {
      const concept = conceptMap.get(node.id);
      if (concept) onSelectConcept(concept);
    },
    [conceptMap, onSelectConcept],
  );

  // Legend of visible edge types
  const visibleEdgeLabels = useMemo(() => {
    const labels = new Set(graphEdges.map((e) => e.label));
    return [...labels].sort();
  }, [graphEdges]);

  return (
    <div className="w-full h-full relative">
      <ReactFlow
        nodes={nodes}
        edges={edges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onNodeClick={onNodeClick}
        nodeTypes={NODE_TYPES}
        fitView
        fitViewOptions={{ padding: 0.3 }}
        minZoom={0.2}
        maxZoom={2}
        proOptions={{ hideAttribution: true }}
      >
        <Controls position="bottom-left" />
        <MiniMap
          nodeColor={(node) => {
            const d = node.data as unknown as ConceptNodeData;
            return d.isSelected ? "hsl(var(--primary))" : "hsl(var(--muted))";
          }}
          maskColor="hsl(var(--background) / 0.7)"
          className="!bg-background !border-border"
        />
        <Background variant={BackgroundVariant.Dots} gap={20} size={1} />

        {/* Edge type legend */}
        {visibleEdgeLabels.length > 0 && (
          <Panel position="top-right">
            <div className="bg-card border border-border rounded-lg p-2.5 shadow-sm">
              <div className="text-[10px] text-muted-foreground font-medium mb-1.5 uppercase tracking-wider">
                Relationships
              </div>
              <div className="flex flex-col gap-1">
                {visibleEdgeLabels.map((label) => (
                  <div key={label} className="flex items-center gap-1.5">
                    <div
                      className="w-3 h-0.5 rounded-full"
                      style={{ backgroundColor: EDGE_COLORS[label] ?? "#888" }}
                    />
                    <span className="text-[11px] text-foreground">
                      {GRAPH_LABEL_DISPLAY[label] ?? label}
                    </span>
                  </div>
                ))}
              </div>
            </div>
          </Panel>
        )}
      </ReactFlow>
    </div>
  );
}
