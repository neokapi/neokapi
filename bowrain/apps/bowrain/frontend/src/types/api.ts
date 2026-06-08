// IOPort is defined once in the shared @neokapi/contract-types package (#817).
import type { IOPort } from "@neokapi/contract-types";
export type { IOPort };

/** BCP-47 locale info with display name */
export interface LocaleInfo {
  code: string;
  display_name: string;
}

/** Data format descriptor */
export interface FormatInfo {
  name: string;
  has_reader: boolean;
  has_writer: boolean;
}

/** Tool descriptor */
export interface ToolInfo {
  name: string;
  description: string;
  category: "transform" | "validate" | "enrich" | "convert" | "pipeline" | "utility";
  hasSchema?: boolean;
  /** Ports the tool reads upstream (non-optional = a requirement). */
  consumes?: IOPort[];
  /** Ports the tool writes. */
  produces?: IOPort[];
  /** True when the tool may be placed in the source-transform stage of a flow. */
  is_source_transform?: boolean;
}

/** Tool parameter schema (matches Go schema.ComponentSchema) */
export interface ToolSchema {
  $id: string;
  title: string;
  description?: string;
  type: string;
  "x-component"?: {
    id: string;
    type: string;
    category?: string;
    displayName?: string;
    description?: string;
  };
  "x-groups"?: Array<{
    id: string;
    label: string;
    description?: string;
    collapsed?: boolean;
    fields: string[];
  }>;
  properties: Record<
    string,
    {
      type: string;
      description?: string;
      default?: unknown;
      enum?: string[];
      "x-widget"?: string;
      "x-placeholder"?: string;
      properties?: Record<string, unknown>;
      items?: { type: string; properties?: Record<string, unknown> };
    }
  >;
}

/** Flow descriptor */
export interface FlowInfo {
  name: string;
  description: string;
}

/** Flow node position */
export interface FlowNodePosition {
  x: number;
  y: number;
}

/** Flow node in a flow definition. The graph is tool nodes only (AD-026); the
 *  I/O ends are bindings, not nodes. */
export interface FlowNodeInfo {
  id: string;
  type: "tool";
  name: string;
  label?: string;
  /** Pipeline stage: "" or undefined = main chain; "source-transform" = leading rewrite stage. */
  stage?: "" | "source-transform";
  config?: Record<string, unknown>;
  position: FlowNodePosition;
}

/** Flow edge in a flow definition */
export interface FlowEdgeInfo {
  id: string;
  source: string;
  target: string;
}

/** Full flow definition for the flow builder */
export interface FlowDefinitionInfo {
  id: string;
  name: string;
  description?: string;
  nodes: FlowNodeInfo[];
  edges: FlowEdgeInfo[];
  source: "built-in" | "user" | "project";
  created_at?: string;
  modified_at?: string;
  /** Source/sink binding intent (AD-026), as wire-format string locators
   *  ("file"/"store"/"none"/"xliff"/…). Nested to avoid colliding with `source`
   *  (the provenance field). Omitted = file binding. */
  binding?: { source?: string; sink?: string };
}

/** Plugin descriptor */
export interface PluginInfo {
  name: string;
  type: string;
  source: string;
  formats: string[];
}

/** User info */
export interface User {
  id: string;
  email: string;
  name: string;
  avatar_url: string;
}

/** Workspace — top-level organizational unit */
export interface Workspace {
  id: string;
  name: string;
  slug: string;
  description: string;
  logo_url: string;
  role: string;
}

/** Membership info */
export interface Membership {
  user_id: string;
  workspace_id: string;
  role: "owner" | "admin" | "member" | "viewer";
  user: User;
}

/** Version info */
export interface VersionInfo {
  version: string;
  commit: string;
  build_date: string;
}

/** Health check response */
export interface HealthResponse {
  status: string;
  version: string;
}

/** Project info */
export interface ProjectInfo {
  id: string;
  name: string;
  default_source_language: string;
  target_languages: string[];
  path: string;
  items: ProjectItem[];
  created_at: string;
  modified_at: string;
}

/** Item within a project */
export interface ProjectItem {
  name: string;
  format: string;
  type: string;
  size: number;
  block_count: number;
  word_count: number;
}

/** Editing constraints applied to a run. Mirrors model.RunConstraints. */
export interface RunConstraints {
  deletable: boolean;
  cloneable: boolean;
  reorderable: boolean;
}

/**
 * One element of a block's inline content sequence (RFC 0001). A Run
 * is a discriminated union keyed by the present field; exactly one of
 * the optional fields is set. Mirrors the Wails backend `RunInfo`.
 */
export interface Run {
  text?: { text: string };
  ph?: {
    id: string;
    type: string;
    subType?: string;
    data: string;
    equiv: string;
    disp?: string;
    constraints?: RunConstraints;
  };
  pcOpen?: {
    id: string;
    type: string;
    subType?: string;
    data: string;
    equiv: string;
    disp?: string;
    constraints?: RunConstraints;
  };
  pcClose?: { id: string; type: string; subType?: string; data: string; equiv?: string };
  sub?: { id: string; ref: string; equiv: string };
  plural?: { pivot: string; forms: Record<string, Run[]> };
  select?: { pivot: string; cases: Record<string, Run[]> };
}

/** Translation block info. Inline markup travels as RFC 0001 Run sequences. */
export interface BlockInfo {
  id: string;
  sourceRuns?: Run[];
  targetRuns?: Record<string, Run[]>;
  translatable: boolean;
  properties: Record<string, string>;
}

/** Update block request */
export interface UpdateBlockRequest {
  project_id: string;
  item_name: string;
  block_id: string;
  target_locale: string;
  text: string;
}

/** Update block target with a structured Run sequence */
export interface UpdateBlockTargetRunsRequest {
  project_id: string;
  item_name: string;
  block_id: string;
  target_locale: string;
  runs: Run[];
}

/** Saved AI provider configuration (managed server-side) */
export interface ProviderConfig {
  id: string;
  name: string;
  provider_type: string;
  model: string;
  base_url: string;
}

/** Provider config with API key for save/test operations */
export interface ProviderConfigWithKey extends ProviderConfig {
  api_key: string;
}

/** Translation stats */
export interface TranslationStats {
  total_blocks: number;
  translated_blocks: number;
  word_count: number;
}

/** Word count result */
export interface WordCountResult {
  source_words: number;
  source_chars: number;
  target_words: Record<string, number>;
  target_chars: Record<string, number>;
}

/** TM entry info */
export interface TMEntryInfo {
  id: string;
  source: string;
  target: string;
  source_locale: string;
  target_locale: string;
  updated_at: string;
}

/** TM search result */
export interface TMSearchResult {
  entries: TMEntryInfo[];
  total_count: number;
}

/** TM update request */
export interface TMUpdateRequest {
  project_id: string;
  entry_id: string;
  source: string;
  target: string;
  source_locale: string;
  target_locale: string;
}

/** Term info */
export interface TermInfo {
  text: string;
  locale: string;
  status: string;
  part_of_speech?: string;
  gender?: string;
  note?: string;
}

/** Concept info */
export interface ConceptInfo {
  id: string;
  domain: string;
  definition: string;
  terms: TermInfo[];
  properties?: Record<string, string>;
  created_at: string;
  updated_at: string;
}

/** Term search result */
export interface TermSearchResult {
  concepts: ConceptInfo[];
  total_count: number;
}

/** Term match info */
export interface TermMatchInfo {
  source_term: string;
  concept_id: string;
  domain: string;
  score: number;
  match_type: string;
  status: string;
  target_terms: TermInfo[];
  position: { start: number; end: number };
}

/** Term lookup result */
export interface TermLookupResult {
  matches: TermMatchInfo[];
}

/** Add concept request */
export interface AddConceptRequest {
  project_id: string;
  domain: string;
  definition: string;
  terms: TermInfo[];
}

/** Update concept request */
export interface UpdateConceptRequest {
  project_id: string;
  concept_id: string;
  domain: string;
  definition: string;
  terms: TermInfo[];
}

/** TM match for a single block */
export interface TMMatchInfo {
  source: string;
  target: string;
  score: number;
  match_type: string;
}

/** Term match for a block */
export interface BlockTermMatch {
  source_term: string;
  target_terms: string[];
  domain: string;
  status: string;
  start: number;
  end: number;
}

/** Term enforcement result */
export interface TermEnforceResult {
  block_id: string;
  source_term: string;
  concept_id: string;
  expected: string[];
  source_text: string;
  target_text: string;
  source_locale: string;
  target_locale: string;
}
