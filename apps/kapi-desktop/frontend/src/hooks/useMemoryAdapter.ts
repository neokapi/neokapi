import { useMemo } from "react";
import { api, call } from "./useApi";
import type {
  MemoryAdapter,
  MemorySearchResult,
  MemoryEntryDTO,
  MemoryPointDTO,
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
/**
 * The memory adapter for one store handle.
 *
 * tabID is what lets the adapter resolve where an entry's unit sits: only a
 * host holding the project's recipe can answer that, and the browser draws no
 * coordinate for one that cannot. A handle with no project tab behind it — an
 * ad-hoc store — omits the resolver rather than guessing.
 */
export function useMemoryAdapter(
  handle: string | null,
  tabID?: string,
  onOpenUnit?: (unitPath: string) => void,
): MemoryAdapter | null {
  return useMemo(() => {
    if (!handle) return null;
    return createWailsMemoryAdapter(handle, tabID, onOpenUnit);
  }, [handle, tabID, onOpenUnit]);
}

/**
 * The unit an entry was approved for, as the file path the context surface
 * resolves against.
 *
 * A unit id is `<document>:<block>`; the document half is the path a governance
 * point is resolved for. An entry that records no unit has no place to resolve.
 */
function unitPath(entry: MemoryEntryDTO): string {
  const unit = entry.unit ?? "";
  const cut = unit.lastIndexOf(":");
  return cut > 0 ? unit.slice(0, cut) : unit;
}

function createWailsMemoryAdapter(
  handle: string,
  tabID?: string,
  onOpenUnit?: (unitPath: string) => void,
): MemoryAdapter {
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
    // Only offered with a project tab behind the store: the resolution is the
    // recipe's, and an ad-hoc store has no recipe to resolve against.
    ...(onOpenUnit
      ? {
          openUnit(entry: MemoryEntryDTO) {
            const path = unitPath(entry);
            if (path) onOpenUnit(path);
          },
        }
      : {}),
    ...(tabID
      ? {
          async resolvePoint(entry: MemoryEntryDTO): Promise<MemoryPointDTO | null> {
            const path = unitPath(entry);
            if (!path) return null;
            const res = await call<{ point?: MemoryPointDTO }>(
              "ContextGoverns",
              tabID,
              "",
              path,
              0,
            );
            const point = res?.point;
            if (!point || (!point.profile && !point.channel && !point.collection)) {
              return null;
            }
            return point;
          },
        }
      : {}),
  };
}
