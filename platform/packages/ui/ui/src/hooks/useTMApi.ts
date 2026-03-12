import { useCallback } from "react";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";
import type { TMSearchResult, TMUpdateRequest, TMEntryInfo } from "../types/api";

export function useTMApi() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  const getTMEntries = useCallback(
    async (
      query: string,
      sourceLocale: string,
      targetLocale: string,
      offset: number,
      limit: number,
    ): Promise<TMSearchResult> => api.getTMEntries(ws, query, sourceLocale, targetLocale, offset, limit),
    [api, ws],
  );

  const getTMCount = useCallback(
    async (): Promise<number> => api.getTMCount(ws),
    [api, ws],
  );

  const addTMEntry = useCallback(
    async (
      source: string,
      target: string,
      sourceLocale: string,
      targetLocale: string,
    ): Promise<TMEntryInfo> => api.addTMEntry(ws, source, target, sourceLocale, targetLocale),
    [api, ws],
  );

  const updateTMEntry = useCallback(
    async (req: TMUpdateRequest): Promise<void> => api.updateTMEntry(ws, req),
    [api, ws],
  );

  const deleteTMEntry = useCallback(
    async (entryId: string): Promise<void> => api.deleteTMEntry(ws, entryId),
    [api, ws],
  );

  return { getTMEntries, getTMCount, addTMEntry, updateTMEntry, deleteTMEntry };
}
