import { useMemo } from "react";
import { api } from "./useApi";
import type {
  TermsAdapter,
  TermSearchResult,
  ConceptDTO,
  AddConceptRequest,
  UpdateConceptRequest,
  ImportResult,
  TermsStats,
} from "@neokapi/ui-primitives";

/** Creates a TermsAdapter that delegates to Wails IPC for a given handle. */
export function useTermsAdapter(handle: string | null): TermsAdapter | null {
  return useMemo(() => {
    if (!handle) return null;
    return createWailsTermsAdapter(handle);
  }, [handle]);
}

function createWailsTermsAdapter(handle: string): TermsAdapter {
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
      const result = await api.importTermsCSVDialog(handle, srcLocale, tgtLocale, domain);
      return (result as ImportResult) ?? { count: 0 };
    },
    async importJSON() {
      const result = await api.importTermsJSONDialog(handle);
      return (result as ImportResult) ?? { count: 0 };
    },
    async exportJSON(name) {
      await api.exportTermsJSONDialog(handle, name);
      return ""; // File saved via native dialog.
    },
    async getStats() {
      const result = await api.getTermsStats(handle);
      return (result as TermsStats) ?? { count: 0 };
    },
  };
}
