import type { Run } from "@neokapi/kapi-format";
import { t } from "@neokapi/i18n-react/runtime";

// --- content memory Types (multilingual model) ---

/**
 * A single language variant of a multilingual content-memory entry.
 * Each content-memory entry has one variant per locale. Inline markup travels as
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
  /** Optional cross-reference to a terms store concept ID. */
  concept_id?: string;
}

/**
 * A context point as its rungs, coarsest first.
 *
 * The stored form joins them with a unit separator, so a surface receives them
 * split rather than parsing a string with control characters in it.
 */
export interface MemoryPointDTO {
  profile?: string;
  channel?: string;
  collection?: string;
}

/** Frontend-facing multilingual content-memory entry. */
export interface MemoryEntryDTO {
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
  /** The block this answer was approved for. */
  unit?: string;
  /**
   * The coordinate the answer was approved at.
   *
   * Recorded only when one source has been answered differently at two points,
   * which is the disagreement the field exists to keep straight. An entry the
   * whole project agrees on carries a unit and no point, and a surface that can
   * resolve one shows where the unit sits instead.
   */
  point?: MemoryPointDTO;
  created_at: string;
  updated_at: string;
}

/**
 * Provenance record — where a content-memory entry came from.
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

export interface MemorySearchResult {
  entries: MemoryEntryDTO[];
  total_count: number;
}

export interface MemoryStats {
  count: number;
  path?: string;
  size?: number;
}

// --- content memory Facets ---

export interface MemoryFacets {
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

/** Filter parameters for faceted content-memory search. */
export interface MemorySearchFilter {
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

export interface MemoryMatchDTO {
  entry: MemoryEntryDTO;
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

export interface LookupMemoryRequest {
  text: string;
  entities: EntityAnnotationDTO[];
  source_locale: string;
  target_locale: string;
  min_score: number;
  max_results: number;
}

/** Input for a single variant when adding / updating a content-memory entry. */
export interface VariantInputDTO {
  text: string;
  runs?: Run[];
}

/** Request payload for adding a multilingual content-memory entry. */
export interface AddMemoryEntryRequest {
  variants: Record<string, VariantInputDTO>;
  hint_src_lang: string;
  project_id?: string;
  note?: string;
  origins?: OriginDTO[];
}

/** Request payload for updating a multilingual content-memory entry. Variants replace wholesale. */
export interface UpdateMemoryEntryRequest {
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

// --- Terms Types ---

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

export interface TermsStats {
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

// Labels are lazy `get` accessors wrapping t() so the dictionary lookup
// happens at render time (after translations load), not at module
// evaluation.
export const ENTITY_TYPES = [
  {
    value: "entity:person",
    get label() {
      return t("Person", "entity type");
    },
  },
  {
    value: "entity:organization",
    get label() {
      return t("Organization", "entity type");
    },
  },
  {
    value: "entity:product",
    get label() {
      return t("Product", "entity type");
    },
  },
  {
    value: "entity:location",
    get label() {
      return t("Location", "entity type");
    },
  },
  {
    value: "entity:date",
    get label() {
      return t("Date", "entity type");
    },
  },
  {
    value: "entity:time",
    get label() {
      return t("Time", "entity type");
    },
  },
  {
    value: "entity:currency",
    get label() {
      return t("Currency", "entity type");
    },
  },
  {
    value: "entity:measurement",
    get label() {
      return t("Measurement", "entity type");
    },
  },
  {
    value: "entity:other",
    get label() {
      return t("Other", "entity type");
    },
  },
] as const;

export type EntityTypeValue = (typeof ENTITY_TYPES)[number]["value"];
