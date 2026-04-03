import { useMemo } from "react";
import { api } from "./useApi";
import type {
  TMAdapter,
  TMSearchResult,
  TMEntryDTO,
  TMMatchDTO,
  AddTMEntryRequest,
  UpdateTMEntryRequest,
  AnnotateEntitiesRequest,
  AnnotateResult,
  LookupTMRequest,
  ImportResult,
  TMStats,
} from "@neokapi/ui-primitives";

/** Creates a TMAdapter that delegates to Wails IPC for a given TM handle. */
export function useTMAdapter(handle: string | null): TMAdapter | null {
  return useMemo(() => {
    if (!handle) return null;
    return createWailsTMAdapter(handle);
  }, [handle]);
}

function createWailsTMAdapter(handle: string): TMAdapter {
  return {
    async search(query, srcLocale, tgtLocale, offset, limit) {
      const result = await api.searchTMEntries(handle, query, srcLocale, tgtLocale, offset, limit);
      return (result as TMSearchResult) ?? { entries: [], total_count: 0 };
    },
    async getEntry(id) {
      return (await api.getTMEntry(handle, id)) as TMEntryDTO | null;
    },
    async addEntry(req: AddTMEntryRequest) {
      await api.addTMEntry(handle, req);
    },
    async updateEntry(req: UpdateTMEntryRequest) {
      await api.updateTMEntry(handle, req);
    },
    async deleteEntry(id) {
      await api.deleteTMEntry(handle, id);
    },
    async deleteEntries(ids) {
      await api.deleteTMEntries(handle, ids);
    },
    async annotateEntities(req: AnnotateEntitiesRequest) {
      const result = await api.annotateEntities(handle, req);
      return (result as AnnotateResult) ?? { entries_updated: 0, entities_added: 0 };
    },
    async lookup(req: LookupTMRequest) {
      const result = await api.lookupTM(handle, req);
      return (result as TMMatchDTO[]) ?? [];
    },
    async importTMX(srcLocale, tgtLocale) {
      return (await api.importTMXDialog(handle, srcLocale, tgtLocale)) as ImportResult | null;
    },
    async exportTMX(srcLocale, tgtLocale) {
      await api.exportTMXDialog(handle, srcLocale, tgtLocale);
    },
    async getStats() {
      const result = await api.getTMStats(handle);
      return (result as TMStats) ?? { count: 0 };
    },
  };
}
