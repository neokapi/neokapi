// Graph-visualization hook for the Brand knowledge graph (AD-021): the concept
// nodes + relation edges the navigator canvas renders, scoped by time/market
// and optionally narrowed to one concept's neighborhood (focus + depth).
import { useQuery } from "@tanstack/react-query";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";
import type { GraphParams } from "../types/brand-graph";

export function useGraph(params?: GraphParams) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  return useQuery({
    queryKey: ["graph", ws, params ?? {}],
    queryFn: () => api.getGraph(ws, params),
    enabled: !!ws,
    staleTime: 15_000,
  });
}
