import type {
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
  TermSearchResult,
  ConceptDTO,
  AddConceptRequest,
  UpdateConceptRequest,
  TermbaseStats,
} from "./types";

/** Adapter interface for TM operations — implemented per-backend (Wails, REST, mock). */
export interface TMAdapter {
  search(
    query: string,
    srcLocale: string,
    tgtLocale: string,
    offset: number,
    limit: number,
  ): Promise<TMSearchResult>;
  getEntry(id: string): Promise<TMEntryDTO | null>;
  addEntry(req: AddTMEntryRequest): Promise<void>;
  updateEntry(req: UpdateTMEntryRequest): Promise<void>;
  deleteEntry(id: string): Promise<void>;
  deleteEntries(ids: string[]): Promise<void>;
  annotateEntities?(req: AnnotateEntitiesRequest): Promise<AnnotateResult>;
  lookup?(req: LookupTMRequest): Promise<TMMatchDTO[]>;
  importTMX?(srcLocale: string, tgtLocale: string): Promise<ImportResult | null>;
  exportTMX?(srcLocale: string, tgtLocale: string): Promise<void>;
  getStats?(): Promise<TMStats>;
}

/** Adapter interface for termbase operations. */
export interface TermbaseAdapter {
  search(
    query: string,
    srcLocale: string,
    tgtLocale: string,
    offset: number,
    limit: number,
  ): Promise<TermSearchResult>;
  getConcept(id: string): Promise<ConceptDTO | null>;
  addConcept(req: AddConceptRequest): Promise<void>;
  updateConcept(req: UpdateConceptRequest): Promise<void>;
  deleteConcept(id: string): Promise<void>;
  deleteConcepts(ids: string[]): Promise<void>;
  importCSV?(
    content: string,
    srcLocale: string,
    tgtLocale: string,
    domain: string,
    hasHeader: boolean,
  ): Promise<ImportResult>;
  importJSON?(content: string): Promise<ImportResult>;
  exportJSON?(name: string): Promise<string>;
  getStats?(): Promise<TermbaseStats>;
}
