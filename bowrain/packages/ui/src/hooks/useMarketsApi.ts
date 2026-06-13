// Market hooks for the Brand knowledge graph (AD-021): workspace-defined scopes
// (a name + the locales it covers) that give validity tags a stable vocabulary.
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";
import type { MarketRequest } from "../types/brand-graph";

function useWs() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  return { api, ws };
}

export function useMarkets() {
  const { api, ws } = useWs();
  return useQuery({
    queryKey: ["markets", ws],
    queryFn: () => api.listMarkets(ws),
    enabled: !!ws,
    staleTime: 60_000,
  });
}

export function useCreateMarket() {
  const { api, ws } = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req: MarketRequest) => api.createMarket(ws, req),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["markets", ws] });
    },
  });
}

export function useUpdateMarket() {
  const { api, ws } = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { marketId: string; req: MarketRequest }) =>
      api.updateMarket(ws, vars.marketId, vars.req),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["markets", ws] });
    },
  });
}

export function useDeleteMarket() {
  const { api, ws } = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (marketId: string) => api.deleteMarket(ws, marketId),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["markets", ws] });
    },
  });
}
