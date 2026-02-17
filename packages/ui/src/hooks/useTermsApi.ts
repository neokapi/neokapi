import { useCallback } from "react";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";
import type {
  TermSearchResult,
  ConceptInfo,
  AddConceptRequest,
  UpdateConceptRequest,
} from "../types/api";

export function useTermsApi() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  const getTerms = useCallback(
    async (
      query: string,
      sourceLocale: string,
      targetLocale: string,
      offset: number,
      limit: number,
    ): Promise<TermSearchResult> => api.getTerms(ws, query, sourceLocale, targetLocale, offset, limit),
    [api, ws],
  );

  const getTermCount = useCallback(
    async (): Promise<number> => api.getTermCount(ws),
    [api, ws],
  );

  const addConcept = useCallback(
    async (req: AddConceptRequest): Promise<ConceptInfo> => api.addConcept(ws, req),
    [api, ws],
  );

  const updateConcept = useCallback(
    async (req: UpdateConceptRequest): Promise<void> => api.updateConcept(ws, req),
    [api, ws],
  );

  const deleteConcept = useCallback(
    async (conceptId: string): Promise<void> => api.deleteConcept(ws, conceptId),
    [api, ws],
  );

  const importTermsCSV = useCallback(
    async (
      csvContent: string,
      sourceLocale: string,
      targetLocale: string,
      domain: string,
      hasHeader: boolean,
    ): Promise<number> => api.importTermsCSV(ws, csvContent, sourceLocale, targetLocale, domain, hasHeader),
    [api, ws],
  );

  const importTermsJSON = useCallback(
    async (jsonContent: string): Promise<number> => api.importTermsJSON(ws, jsonContent),
    [api, ws],
  );

  const exportTermsJSON = useCallback(
    async (name: string): Promise<string> => api.exportTermsJSON(ws, name),
    [api, ws],
  );

  return {
    getTerms, getTermCount,
    addConcept, updateConcept, deleteConcept,
    importTermsCSV, importTermsJSON, exportTermsJSON,
  };
}
