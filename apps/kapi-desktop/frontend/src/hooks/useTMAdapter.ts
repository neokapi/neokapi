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
  TMFacets,
  ImportSessionDTO,
} from "@neokapi/ui-primitives";

const EMPTY_FACETS: TMFacets = {
  locales: [],
  projects: [],
  entity_types: [],
  import_sessions: [],
  has_codes: 0,
  no_codes: 0,
};

/** Creates a TMAdapter that delegates to Wails IPC for a given TM handle. */
export function useTMAdapter(handle: string | null): TMAdapter | null {
  return useMemo(() => {
    if (!handle) return null;
    return createWailsTMAdapter(handle);
  }, [handle]);
}

function createWailsTMAdapter(handle: string): TMAdapter {
  return {
    async search(query, anyLocale, requireLocale, offset, limit) {
      const result = await api.searchTMEntries(
        handle,
        query,
        anyLocale,
        requireLocale,
        offset,
        limit,
      );
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
    async importTMX() {
      return (await api.importTMXDialog(handle)) as ImportResult | null;
    },
    async exportTMX(locales: string[]) {
      await api.exportTMXDialog(handle, locales);
    },
    async getStats() {
      const result = await api.getTMStats(handle);
      return (result as TMStats) ?? { count: 0 };
    },
    async getFacets() {
      const result = await api.getTMFacets(handle);
      return (result as TMFacets) ?? EMPTY_FACETS;
    },
    async getFacetsFiltered(query, anyLocale, requireLocale, filter) {
      const result = await api.getTMFacetsFiltered(handle, query, anyLocale, requireLocale, filter);
      return (result as TMFacets) ?? EMPTY_FACETS;
    },
    async searchFiltered(query, anyLocale, requireLocale, filter, offset, limit) {
      const result = await api.searchTMEntriesFiltered(
        handle,
        query,
        anyLocale,
        requireLocale,
        filter,
        offset,
        limit,
      );
      return (result as TMSearchResult) ?? { entries: [], total_count: 0 };
    },
    async listImportSessions() {
      const result = await api.listTMImportSessions(handle);
      return (result as ImportSessionDTO[]) ?? [];
    },
    async getImportSession(id: string) {
      return (await api.getTMImportSession(handle, id)) as ImportSessionDTO | null;
    },
    async deleteImportSession(id: string) {
      await api.deleteTMImportSession(handle, id);
    },
  };
}
