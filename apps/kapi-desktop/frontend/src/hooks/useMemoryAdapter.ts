import { useMemo } from "react";
import { api } from "./useApi";
import type {
  MemoryAdapter,
  MemorySearchResult,
  MemoryEntryDTO,
  MemoryMatchDTO,
  AddMemoryEntryRequest,
  UpdateMemoryEntryRequest,
  AnnotateEntitiesRequest,
  AnnotateResult,
  LookupMemoryRequest,
  MemoryStats,
  MemoryFacets,
  ImportSessionDTO,
} from "@neokapi/ui-primitives";

const EMPTY_FACETS: MemoryFacets = {
  locales: [],
  projects: [],
  entity_types: [],
  import_sessions: [],
  has_codes: 0,
  no_codes: 0,
};

/** Creates a MemoryAdapter that delegates to Wails IPC for a given content-memory handle. */
export function useMemoryAdapter(handle: string | null): MemoryAdapter | null {
  return useMemo(() => {
    if (!handle) return null;
    return createWailsMemoryAdapter(handle);
  }, [handle]);
}

function createWailsMemoryAdapter(handle: string): MemoryAdapter {
  return {
    async search(query, anyLocale, requireLocale, offset, limit) {
      const result = await api.searchMemoryEntries(
        handle,
        query,
        anyLocale,
        requireLocale,
        offset,
        limit,
      );
      return (result as MemorySearchResult) ?? { entries: [], total_count: 0 };
    },
    async getEntry(id) {
      return (await api.getMemoryEntry(handle, id)) as MemoryEntryDTO | null;
    },
    async addEntry(req: AddMemoryEntryRequest) {
      await api.addMemoryEntry(handle, req);
    },
    async updateEntry(req: UpdateMemoryEntryRequest) {
      await api.updateMemoryEntry(handle, req);
    },
    async deleteEntry(id) {
      await api.deleteMemoryEntry(handle, id);
    },
    async deleteEntries(ids) {
      await api.deleteMemoryEntries(handle, ids);
    },
    async annotateEntities(req: AnnotateEntitiesRequest) {
      const result = await api.annotateEntities(handle, req);
      return (result as AnnotateResult) ?? { entries_updated: 0, entities_added: 0 };
    },
    async lookup(req: LookupMemoryRequest) {
      const result = await api.lookupMemory(handle, req);
      return (result as MemoryMatchDTO[]) ?? [];
    },
    async getStats() {
      const result = await api.getMemoryStats(handle);
      return (result as MemoryStats) ?? { count: 0 };
    },
    async getFacets() {
      const result = await api.getMemoryFacets(handle);
      return (result as MemoryFacets) ?? EMPTY_FACETS;
    },
    async getFacetsFiltered(query, anyLocale, requireLocale, filter) {
      const result = await api.getMemoryFacetsFiltered(
        handle,
        query,
        anyLocale,
        requireLocale,
        filter,
      );
      return (result as MemoryFacets) ?? EMPTY_FACETS;
    },
    async searchFiltered(query, anyLocale, requireLocale, filter, offset, limit) {
      const result = await api.searchMemoryEntriesFiltered(
        handle,
        query,
        anyLocale,
        requireLocale,
        filter,
        offset,
        limit,
      );
      return (result as MemorySearchResult) ?? { entries: [], total_count: 0 };
    },
    async listImportSessions() {
      const result = await api.listMemoryImportSessions(handle);
      return (result as ImportSessionDTO[]) ?? [];
    },
    async getImportSession(id: string) {
      return (await api.getMemoryImportSession(handle, id)) as ImportSessionDTO | null;
    },
    async deleteImportSession(id: string) {
      await api.deleteMemoryImportSession(handle, id);
    },
  };
}
