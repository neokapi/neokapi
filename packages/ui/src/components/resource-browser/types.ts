import type { SpanInfo } from "../../types/span";

// --- TM Types ---

export interface TMEntryDTO {
  id: string;
  source_text: string;
  target_text: string;
  source_coded: string;
  target_coded: string;
  source_spans: SpanInfo[];
  target_spans: SpanInfo[];
  source_locale: string;
  target_locale: string;
  project_id: string;
  properties?: Record<string, string>;
  created_at: string;
  updated_at: string;
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

export interface AddTMEntryRequest {
  source: string;
  target: string;
  source_locale: string;
  target_locale: string;
  project_id?: string;
}

export interface UpdateTMEntryRequest {
  entry_id: string;
  source: string;
  target: string;
  target_coded?: string;
  target_spans?: SpanInfo[];
  source_locale: string;
  target_locale: string;
  project_id?: string;
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
