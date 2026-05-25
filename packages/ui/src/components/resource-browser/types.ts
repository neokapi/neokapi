import type { Run } from "@neokapi/kapi-format";

// --- TM Types (multilingual model) ---

/**
 * A single language variant of a multilingual TM entry.
 * Each TM entry has one variant per locale. Inline markup travels as
 * an RFC 0001 Run sequence; `text` is the flattened plain form.
 */
export interface VariantDTO {
  locale: string;
  text: string;
  runs: Run[];
}

/** Position + text for a single-locale entity value. */
export interface EntityValueDTO {
  text: string;
  start: number;
  end: number;
}

/** Multilingual entity mapping — one placeholder, per-locale values. */
export interface EntityMappingDTO {
  placeholder_id: string;
  type: string;
  values: Record<string, EntityValueDTO>;
  /** Optional cross-reference to a termbase concept ID. */
  concept_id?: string;
}

/** Frontend-facing multilingual TM entry. */
export interface TMEntryDTO {
  id: string;
  project_id: string;
  /** Per-locale variants keyed by locale tag (e.g. "en-US"). */
  variants: Record<string, VariantDTO>;
  /** Locale pointer used as the default source for display. */
  hint_src_lang: string;
  entities?: EntityMappingDTO[];
  properties?: Record<string, string>;
  note?: string;
  origins?: OriginDTO[];
  created_at: string;
  updated_at: string;
}

/**
 * Provenance record — where a TM entry came from.
 * An entry can have multiple origins if the same source was ingested
 * from multiple locations. When the origin came from an import session
 * the `session_id` links back to an ImportSessionDTO.
 */
export interface OriginDTO {
  source: string; // "file" | "tool" | "import" | "user"
  key?: string; // file path + key, tool name, etc.
  reference?: string; // commit hash, job ID, URL
  added_at: string; // ISO 8601
  added_by?: string; // user ID or tool name
  session_id?: string; // link to an ImportSessionDTO when applicable
}

export interface TMSearchResult {
  entries: TMEntryDTO[];
  total_count: number;
}

export interface TMStats {
  count: number;
  path?: string;
  size?: number;
}

// --- TM Facets ---

export interface TMFacets {
  locales: LocaleFacet[];
  projects: ProjectFacet[];
  entity_types: EntityTypeFacet[];
  import_sessions: ImportSessionFacet[];
  has_codes: number;
  no_codes: number;
}

/** Single-locale variant count (replaces the legacy locale-pair facet). */
export interface LocaleFacet {
  locale: string;
  count: number;
}

export interface ProjectFacet {
  project_id: string;
  count: number;
}

export interface EntityTypeFacet {
  type: string;
  count: number;
}

/** Compact import-session record used as a sidebar facet. */
export interface ImportSessionFacet {
  session_id: string;
  file_key: string;
  tool_name?: string;
  imported_at: string;
  count: number;
}

/** Full import-session record fetched on demand from the backend. */
export interface ImportSessionDTO {
  id: string;
  file_key: string;
  file_hash: string;
  file_size_bytes: number;
  imported_at: string;
  imported_by: string;
  tool_name: string;
  tool_version: string;
  seg_type: string;
  admin_lang: string;
  src_lang: string;
  data_type: string;
  original_format: string;
  original_encoding: string;
  entry_count: number;
  properties?: Record<string, string>;
}

/** Filter parameters for faceted TM search. */
export interface TMSearchFilter {
  project_id?: string;
  /** Restrict to entries that originated from these import sessions. */
  session_ids?: string[];
  entity_types?: string[];
  entity_values?: EntityValueFilter[];
  has_codes?: boolean | null;
}

/** Filter for searching by a specific entity value + type. */
export interface EntityValueFilter {
  value: string;
  type: string; // e.g. "entity:person"
}

export interface TMMatchDTO {
  entry: TMEntryDTO;
  score: number;
  match_type: string;
  entity_adaptations?: EntityAdaptationDTO[];
}

export interface EntityAdaptationDTO {
  placeholder_id: string;
  type: string;
  stored_value: string;
  current_value: string;
}

export interface EntityAnnotationDTO {
  text: string;
  type: string;
  start: number;
  end: number;
}

export interface LookupTMRequest {
  text: string;
  entities: EntityAnnotationDTO[];
  source_locale: string;
  target_locale: string;
  min_score: number;
  max_results: number;
}

/** Input for a single variant when adding / updating a TM entry. */
export interface VariantInputDTO {
  text: string;
  runs?: Run[];
}

/** Request payload for adding a multilingual TM entry. */
export interface AddTMEntryRequest {
  variants: Record<string, VariantInputDTO>;
  hint_src_lang: string;
  project_id?: string;
  note?: string;
  origins?: OriginDTO[];
}

/** Request payload for updating a multilingual TM entry. Variants replace wholesale. */
export interface UpdateTMEntryRequest {
  entry_id: string;
  variants: Record<string, VariantInputDTO>;
  hint_src_lang: string;
  project_id?: string;
  note?: string;
  origins?: OriginDTO[];
}

export interface AnnotateEntitiesRequest {
  entry_ids: string[];
  patterns: EntityPatternRequest[];
}

export interface EntityPatternRequest {
  text: string;
  entity_type: string;
  case_sensitive: boolean;
}

export interface AnnotateResult {
  entries_updated: number;
  entities_added: number;
}

// --- Termbase Types ---

export interface ConceptDTO {
  id: string;
  project_id: string;
  domain: string;
  definition: string;
  source: string;
  terms: TermDTO[];
  properties?: Record<string, string>;
  created_at: string;
  updated_at: string;
}

export interface TermDTO {
  text: string;
  locale: string;
  status: string;
  part_of_speech?: string;
  gender?: string;
  note?: string;
  competitor_term?: boolean;
}

export interface TermSearchResult {
  concepts: ConceptDTO[];
  total_count: number;
}

export interface TermbaseStats {
  count: number;
  path?: string;
}

export interface AddConceptRequest {
  project_id?: string;
  domain: string;
  definition: string;
  terms: TermDTO[];
}

export interface UpdateConceptRequest {
  concept_id: string;
  project_id?: string;
  domain: string;
  definition: string;
  terms: TermDTO[];
}

// --- Shared ---

export interface ImportResult {
  session_id: string;
  count: number;
}

export interface ResourceInfo {
  name: string;
  path: string;
  size: number;
  modified: string;
}

// --- Entity constants ---

export const ENTITY_TYPES = [
  { value: "entity:person", label: "Person" },
  { value: "entity:organization", label: "Organization" },
  { value: "entity:product", label: "Product" },
  { value: "entity:location", label: "Location" },
  { value: "entity:date", label: "Date" },
  { value: "entity:time", label: "Time" },
  { value: "entity:currency", label: "Currency" },
  { value: "entity:measurement", label: "Measurement" },
  { value: "entity:other", label: "Other" },
] as const;

export type EntityTypeValue = (typeof ENTITY_TYPES)[number]["value"];
