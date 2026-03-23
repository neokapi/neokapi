import { useCallback } from "react";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";
import type { ConceptHierarchyNode, GraphNode, GraphEdge, GraphPath } from "../types/api";

export function useGraphApi() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  const getConceptHierarchy = useCallback(
    async (): Promise<ConceptHierarchyNode[]> => api.getConceptHierarchy(ws),
    [api, ws],
  );

  const getGraphNeighbors = useCallback(
    async (
      nodeId: string,
      direction?: "outgoing" | "incoming" | "both",
      label?: string,
    ): Promise<GraphNode[]> => api.getGraphNeighbors(ws, nodeId, direction, label),
    [api, ws],
  );

  const getGraphEdges = useCallback(
    async (nodeId: string, direction?: "outgoing" | "incoming" | "both"): Promise<GraphEdge[]> =>
      api.getGraphEdges(ws, nodeId, direction),
    [api, ws],
  );

  const getGraphShortestPath = useCallback(
    async (fromId: string, toId: string): Promise<GraphPath> =>
      api.getGraphShortestPath(ws, fromId, toId),
    [api, ws],
  );

  return {
    getConceptHierarchy,
    getGraphNeighbors,
    getGraphEdges,
    getGraphShortestPath,
  };
}
