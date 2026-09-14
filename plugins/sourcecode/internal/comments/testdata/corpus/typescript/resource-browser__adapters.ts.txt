import type {
  MemorySearchResult,
  MemoryEntryDTO,
  MemoryPointDTO,
  MemoryMatchDTO,
  AddMemoryEntryRequest,
  UpdateMemoryEntryRequest,
  AnnotateEntitiesRequest,
  AnnotateResult,
  LookupMemoryRequest,
  ImportResult,
  MemoryStats,
  MemoryFacets,
  MemorySearchFilter,
  ImportSessionDTO,
  TermSearchResult,
  ConceptDTO,
  AddConceptRequest,
  UpdateConceptRequest,
  TermsStats,
} from "./types";

/** Adapter interface for Memory operations — implemented per-backend (Wails, REST, mock). */
export interface MemoryAdapter {
  /**
   * Plain search.
   * @param query     full-text search query
   * @param anyLocale restrict search scope to variants in this locale (empty = any)
   * @param requireLocale require entries to have a variant in this locale (empty = none)
   */
  search(
    query: string,
    anyLocale: string,
    requireLocale: string,
    offset: number,
    limit: number,
  ): Promise<MemorySearchResult>;
  getEntry(id: string): Promise<MemoryEntryDTO | null>;
  addEntry(req: AddMemoryEntryRequest): Promise<void>;
  updateEntry(req: UpdateMemoryEntryRequest): Promise<void>;
  deleteEntry(id: string): Promise<void>;
  deleteEntries(ids: string[]): Promise<void>;
  annotateEntities?(req: AnnotateEntitiesRequest): Promise<AnnotateResult>;
  lookup?(req: LookupMemoryRequest): Promise<MemoryMatchDTO[]>;
  /** Launches a file dialog and imports a TMX file. Returns null on cancel. */
  importTMX?(): Promise<ImportResult | null>;
  /** Launches a save dialog and exports the specified locales (empty = all). */
  exportTMX?(locales: string[]): Promise<void>;
  getStats?(): Promise<MemoryStats>;
  getFacets?(): Promise<MemoryFacets>;
  /** Facet counts scoped to the current search query and filter. */
  getFacetsFiltered?(
    query: string,
    anyLocale: string,
    requireLocale: string,
    filter: MemorySearchFilter,
  ): Promise<MemoryFacets>;
  searchFiltered?(
    query: string,
    anyLocale: string,
    requireLocale: string,
    filter: MemorySearchFilter,
    offset: number,
    limit: number,
  ): Promise<MemorySearchResult>;
  /** List every import session row (most recent first). */
  listImportSessions?(): Promise<ImportSessionDTO[]>;
  /** Fetch a single import session by ID. */
  getImportSession?(id: string): Promise<ImportSessionDTO | null>;
  /** Delete an import session row (origins keep pointing at empty session_id). */
  deleteImportSession?(id: string): Promise<void>;
  /**
   * The coordinate the unit an entry was approved for sits at.
   *
   * Only a host holding the project's recipe can answer this: the corpus stores
   * a point for a contested entry and nothing for the rest, because an answer
   * every point agrees on is not a decision about a place. A host that has the
   * recipe resolves where the unit sits; one that does not omits the method, and
   * the browser shows no coordinate rather than guessing at one.
   */
  resolvePoint?(entry: MemoryEntryDTO): Promise<MemoryPointDTO | null>;
  /**
   * Open the project's context surface standing at an entry's unit. Absent
   * where there is no such surface to open, and the unit then reads as a label
   * rather than a link.
   */
  openUnit?(entry: MemoryEntryDTO): void;
}

/** Adapter interface for terms operations. */
export interface TermsAdapter {
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
  getStats?(): Promise<TermsStats>;
}
