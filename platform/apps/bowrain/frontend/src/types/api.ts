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
  category: "transform" | "validate" | "enrich" | "utility";
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

/** Flow node in a flow definition */
export interface FlowNodeInfo {
  id: string;
  type: "tool" | "reader" | "writer";
  name: string;
  label?: string;
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
  source: string;
  created_at?: string;
  modified_at?: string;
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
  source_locale: string;
  target_locales: string[];
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

/** Inline span info */
export interface SpanInfo {
  span_type: "opening" | "closing" | "placeholder";
  type: string;
  id: string;
  data: string;
}

/** Translation block info */
export interface BlockInfo {
  id: string;
  source: string;
  source_coded?: string;
  source_spans?: SpanInfo[];
  targets: Record<string, string>;
  targets_coded?: Record<string, string>;
  translatable: boolean;
  has_spans: boolean;
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

/** Update block target with coded text and spans */
export interface UpdateBlockTargetCodedRequest {
  project_id: string;
  item_name: string;
  block_id: string;
  target_locale: string;
  coded_text: string;
  spans: SpanInfo[];
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
