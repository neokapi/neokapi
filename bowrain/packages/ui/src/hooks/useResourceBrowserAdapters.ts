import { useMemo } from "react";
import type {
  MemoryAdapter,
  TermsAdapter,
  MemoryEntryDTO,
  MemorySearchResult,
  AddMemoryEntryRequest,
  UpdateMemoryEntryRequest,
  ConceptDTO,
  TermDTO,
  TermSearchResult,
  AddConceptRequest,
  UpdateConceptRequest,
  ImportResult,
  TermsStats,
} from "@neokapi/ui-primitives";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";
import type { ApiAdapter } from "../api/adapter";
import type { MemoryEntryInfo, ConceptInfo, TermInfo } from "../types/api";

// Bowrain's content memory is stored as bilingual (source + target pair)
// entries, whereas the shared @neokapi/ui-primitives MemoryBrowser speaks the
// framework's multilingual model (one variant per locale). These adapters
// bridge the two: each bowrain MemoryEntryInfo becomes a two-variant MemoryEntryDTO,
// and multilingual add/update requests are reduced back to a source/target
// pair using the entry's `hint_src_lang`. The terms store model matches the
// shared ConceptDTO shape directly, so that adapter is a thin projection.

function memoryEntryToDTO(e: MemoryEntryInfo): MemoryEntryDTO {
  return {
    id: e.id,
    project_id: e.project_id ?? "",
    variants: {
      [e.source_language]: {
        locale: e.source_language,
        text: e.source,
        // Carry the text as a run so that MemoryBrowser's per-variant edit path
        // (which rebuilds every variant from its runs) preserves the
        // unchanged locale instead of flattening it to "".
        runs: e.source ? [{ text: e.source }] : [],
      },
      [e.target_language]: {
        locale: e.target_language,
        text: e.target,
        runs: e.target ? [{ text: e.target }] : [],
      },
    },
    hint_src_lang: e.source_language,
    created_at: e.updated_at,
    updated_at: e.updated_at,
  };
}

/** Pick the source/target locale + text pair out of a multilingual request. */
function pairFromVariants(
  variants: UpdateMemoryEntryRequest["variants"],
  hintSrcLang: string,
): { source: string; target: string; sourceLocale: string; targetLocale: string } {
  const locales = Object.keys(variants);
  const sourceLocale = hintSrcLang && variants[hintSrcLang] ? hintSrcLang : (locales[0] ?? "");
  const targetLocale = locales.find((l) => l !== sourceLocale) ?? "";
  return {
    source: variants[sourceLocale]?.text ?? "",
    target: variants[targetLocale]?.text ?? "",
    sourceLocale,
    targetLocale,
  };
}

function createMemoryAdapter(
  api: ApiAdapter,
  ws: string,
  defaultSource: string,
  defaultTargets: string[],
): MemoryAdapter {
  return {
    async search(query, anyLocale, requireLocale, offset, limit): Promise<MemorySearchResult> {
      const sourceLocale = anyLocale || defaultSource;
      const targetLocale = requireLocale || defaultTargets[0] || "";
      const result = await api.getMemoryEntries(ws, query, sourceLocale, targetLocale, offset, limit);
      return {
        entries: (result.entries ?? []).map(memoryEntryToDTO),
        total_count: result.total_count,
      };
    },
    // Not driven by MemoryBrowser — the row already holds the loaded entry.
    async getEntry(): Promise<MemoryEntryDTO | null> {
      return null;
    },
    async addEntry(req: AddMemoryEntryRequest): Promise<void> {
      const { source, target, sourceLocale, targetLocale } = pairFromVariants(
        req.variants,
        req.hint_src_lang,
      );
      await api.addMemoryEntry(ws, source, target, sourceLocale, targetLocale);
    },
    async updateEntry(req: UpdateMemoryEntryRequest): Promise<void> {
      const { source, target, sourceLocale, targetLocale } = pairFromVariants(
        req.variants,
        req.hint_src_lang,
      );
      await api.updateMemoryEntry(ws, {
        project_id: req.project_id ?? "",
        entry_id: req.entry_id,
        source,
        target,
        source_locale: sourceLocale,
        target_locale: targetLocale,
      });
    },
    async deleteEntry(id: string): Promise<void> {
      await api.deleteMemoryEntry(ws, id);
    },
    async deleteEntries(ids: string[]): Promise<void> {
      await Promise.all(ids.map((id) => api.deleteMemoryEntry(ws, id)));
    },
  };
}

function conceptToDTO(c: ConceptInfo): ConceptDTO {
  return {
    id: c.id,
    project_id: c.project_id ?? "",
    domain: c.domain,
    definition: c.definition,
    source: "",
    terms: c.terms.map((t) => ({
      text: t.text,
      locale: t.locale,
      status: t.status,
      part_of_speech: t.part_of_speech,
      gender: t.gender,
      note: t.note,
    })),
    properties: c.properties,
    created_at: c.created_at,
    updated_at: c.updated_at,
  };
}

function termsToInfo(terms: TermDTO[]): TermInfo[] {
  return terms.map((t) => ({
    text: t.text,
    locale: t.locale,
    status: t.status,
    part_of_speech: t.part_of_speech,
    gender: t.gender,
    note: t.note,
  }));
}

function createTermsAdapter(api: ApiAdapter, ws: string): TermsAdapter {
  return {
    async search(query, srcLocale, tgtLocale, offset, limit): Promise<TermSearchResult> {
      const result = await api.getTerms(ws, query, srcLocale, tgtLocale, offset, limit);
      return {
        concepts: (result.concepts ?? []).map(conceptToDTO),
        total_count: result.total_count,
      };
    },
    // Not driven by TermsBrowser — the card already holds the loaded concept.
    async getConcept(): Promise<ConceptDTO | null> {
      return null;
    },
    async addConcept(req: AddConceptRequest): Promise<void> {
      await api.addConcept(ws, {
        project_id: req.project_id,
        domain: req.domain,
        definition: req.definition,
        terms: termsToInfo(req.terms),
      });
    },
    async updateConcept(req: UpdateConceptRequest): Promise<void> {
      await api.updateConcept(ws, {
        project_id: req.project_id ?? "",
        concept_id: req.concept_id,
        domain: req.domain,
        definition: req.definition,
        terms: termsToInfo(req.terms),
      });
    },
    async deleteConcept(id: string): Promise<void> {
      await api.deleteConcept(ws, id);
    },
    async deleteConcepts(ids: string[]): Promise<void> {
      await Promise.all(ids.map((id) => api.deleteConcept(ws, id)));
    },
    async importCSV(content, srcLocale, tgtLocale, domain, hasHeader): Promise<ImportResult> {
      const count = await api.importTermsCSV(ws, content, srcLocale, tgtLocale, domain, hasHeader);
      return { session_id: "", count };
    },
    async importJSON(content: string): Promise<ImportResult> {
      const count = await api.importTermsJSON(ws, content);
      return { session_id: "", count };
    },
    async exportJSON(name: string): Promise<string> {
      return api.exportTermsJSON(ws, name);
    },
    async getStats(): Promise<TermsStats> {
      return { count: await api.getTermCount(ws) };
    },
  };
}

/**
 * Builds a {@link MemoryAdapter} backed by the bowrain REST {@link ApiAdapter} for
 * the active workspace. `defaultSource`/`defaultTargets` seed the bilingual
 * pair the bowrain backend requires when the browser has not yet scoped a
 * locale.
 */
export function useMemoryBrowserAdapter(
  defaultSource: string,
  defaultTargets: string[],
): MemoryAdapter | null {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  const targetKey = defaultTargets.join(",");
  return useMemo(() => {
    if (!ws) return null;
    return createMemoryAdapter(api, ws, defaultSource, targetKey ? targetKey.split(",") : []);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [api, ws, defaultSource, targetKey]);
}

/** Builds a {@link TermsAdapter} backed by the bowrain REST adapter. */
export function useTermsBrowserAdapter(): TermsAdapter | null {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  return useMemo(() => {
    if (!ws) return null;
    return createTermsAdapter(api, ws);
  }, [api, ws]);
}
