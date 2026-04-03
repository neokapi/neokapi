import { useMemo } from "react";
import { api } from "./useApi";
import type {
  TermbaseAdapter,
  TermSearchResult,
  ConceptDTO,
  AddConceptRequest,
  UpdateConceptRequest,
  ImportResult,
  TermbaseStats,
} from "@neokapi/ui-primitives";

/** Creates a TermbaseAdapter that delegates to Wails IPC for a given handle. */
export function useTermbaseAdapter(handle: string | null): TermbaseAdapter | null {
  return useMemo(() => {
    if (!handle) return null;
    return createWailsTermbaseAdapter(handle);
  }, [handle]);
}

function createWailsTermbaseAdapter(handle: string): TermbaseAdapter {
  return {
    async search(query, srcLocale, tgtLocale, offset, limit) {
      const result = await api.searchTerms(handle, query, srcLocale, tgtLocale, offset, limit);
      return (result as TermSearchResult) ?? { concepts: [], total_count: 0 };
    },
    async getConcept(id) {
      return (await api.getConcept(handle, id)) as ConceptDTO | null;
    },
    async addConcept(req: AddConceptRequest) {
      await api.addConcept(handle, req);
    },
    async updateConcept(req: UpdateConceptRequest) {
      await api.updateConcept(handle, req);
    },
    async deleteConcept(id) {
      await api.deleteConcept(handle, id);
    },
    async deleteConcepts(ids) {
      await api.deleteConcepts(handle, ids);
    },
    async importCSV(_content, srcLocale, tgtLocale, domain) {
      const result = await api.importTermbaseCSVDialog(handle, srcLocale, tgtLocale, domain);
      return (result as ImportResult) ?? { count: 0 };
    },
    async importJSON() {
      const result = await api.importTermbaseJSONDialog(handle);
      return (result as ImportResult) ?? { count: 0 };
    },
    async exportJSON(name) {
      await api.exportTermbaseJSONDialog(handle, name);
      return ""; // File saved via native dialog.
    },
    async getStats() {
      const result = await api.getTermbaseStats(handle);
      return (result as TermbaseStats) ?? { count: 0 };
    },
  };
}
