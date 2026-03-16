/** User info from auth system */
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
  type: "personal" | "team";
  languages?: string[];
  role: string; // current user's role in the workspace
}

/** Membership — ties a user to a workspace */
export interface Membership {
  user_id: string;
  workspace_id: string;
  role: "owner" | "admin" | "member" | "viewer";
  user: User;
}

/** API token (stored; no plaintext) */
export interface ApiToken {
  id: string;
  user_id: string;
  workspace_id: string;
  name: string;
  token_prefix: string;
  scopes: string;
  last_used_at: string | null;
  expires_at: string | null;
  created_at: string;
}

/** API token creation response — includes the plaintext token (shown once) */
export interface CreateApiTokenResponse {
  id: string;
  name: string;
  token_prefix: string;
  token: string;
  scopes: string;
  expires_at: string | null;
  created_at: string;
}

/** Workspace invite */
export interface Invite {
  id: string;
  workspace_id: string;
  code: string;
  email: string;
  role: "owner" | "admin" | "member" | "viewer";
  max_uses: number;
  use_count: number;
  created_by: string;
  expires_at: string;
  created_at: string;
}

/** Accept invite response */
export interface AcceptInviteResponse {
  workspace_id: string;
  workspace_slug: string;
  workspace_name: string;
  role: string;
}

/** Claim project response */
export interface ClaimProjectResponse {
  project_id: string;
  workspace_slug: string;
}

/** Stream visibility */
export type StreamVisibility = "public" | "private" | "shared";

/** Stream info */
export interface StreamInfo {
  name: string;
  parent: string;
  base_cursor: number;
  archived: boolean;
  visibility: StreamVisibility;
  description: string;
  created_at: string;
  created_by: string;
  shared_with?: string[];
}

/** Stream diff block change */
export interface BlockChangeInfo {
  block_id: string;
  change_type: "added" | "modified" | "removed";
  old_hash: string;
  new_hash: string;
}

/** Stream diff result */
export interface StreamDiffResult {
  stream_name: string;
  parent_name: string;
  changes: BlockChangeInfo[];
}

/** Stream merge result */
export interface StreamMergeResult {
  merged_blocks: number;
  added_blocks: number;
  modified_blocks: number;
  removed_blocks: number;
}

/** Create stream request */
export interface CreateStreamRequest {
  name: string;
  parent?: string;
  visibility?: StreamVisibility;
  description?: string;
}

/** Collection kind */
export type CollectionKind = "uploaded" | "connected";

/** Collection info */
export interface CollectionInfo {
  id: string;
  project_id: string;
  name: string;
  kind: CollectionKind;
  item_label: string;
  is_default: boolean;
  stream?: string;
  connector_config?: Record<string, string>;
  item_count: number;
  created_at: string;
  updated_at: string;
}

/** Create collection request */
export interface CreateCollectionRequest {
  name: string;
  kind: CollectionKind;
  item_label?: string;
  stream?: string;
  connector_config?: Record<string, string>;
}

/** Archived project info (from the bin) */
export interface ArchivedProject {
  id: string;
  name: string;
  default_source_language: string;
  target_languages: string[];
  target_language_mode?: string;
  workspace_id?: string;
  archived: boolean;
  archived_at?: string;
  created_at: string;
  updated_at: string;
}

/** Project info */
export interface ProjectInfo {
  id: string;
  name: string;
  default_source_language: string;
  target_languages: string[];
  target_language_mode?: string;
  workspace_id?: string;
  path?: string;
  items?: ProjectItem[];
  collections?: CollectionInfo[];
  streams?: StreamInfo[];
  active_stream?: string;
  created_at: string;
  modified_at: string;
}

/** Item within a project */
export interface ProjectItem {
  id: string;
  name: string;
  format: string;
  type: string;
  collection_id?: string;
  size: number;
  block_count: number;
  word_count: number;
}

/** Audit log entry */
export interface AuditEntry {
  id: number;
  project_id: string;
  event_type: string;
  actor: string;
  source: string;
  data: string; // JSON string
  created_at: string;
}

/** Audit log query parameters */
export interface AuditQuery {
  project?: string;
  type?: string;
  actor?: string;
  search?: string;
  limit?: number;
  offset?: number;
}

/** Server config response */
export interface ConfigResponse {
  mode: "standalone" | "server";
  version: string;
  commit: string;
  build_date: string;
}

/** Static version.json served alongside the web SPA */
export interface WebVersionInfo {
  version: string;
  commit: string;
  build_date: string;
  component: string;
}

// ---------------------------------------------------------------------------
// Editor & Block types
// ---------------------------------------------------------------------------

/** Inline span info */
export interface SpanInfo {
  span_type: "opening" | "closing" | "placeholder";
  type: string;
  sub_type?: string;
  id: string;
  data: string;
  display_text?: string;
  equiv_text?: string;
  deletable?: boolean;
  cloneable?: boolean;
  can_reorder?: boolean;
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
  entities?: EntityInfo[];
}

/** Entity annotation on a block */
export interface EntityInfo {
  key: string;
  text: string;
  type: string;
  start: number;
  end: number;
  dnt: boolean;
  source?: string;
  locale?: string;
}

/** Notification info */
export interface NotificationInfo {
  id: string;
  user_id: string;
  type: string;
  title: string;
  body: string;
  project_id?: string;
  link_url?: string;
  read: boolean;
  created_at: string;
}

/** Update block request */
export interface UpdateBlockRequest {
  project_id: string;
  item_name: string;
  block_id: string;
  target_locale: string;
  text: string;
  stream?: string;
}

/** Update block target with coded text and spans */
export interface UpdateBlockTargetCodedRequest {
  project_id: string;
  item_name: string;
  block_id: string;
  target_locale: string;
  coded_text: string;
  spans: SpanInfo[];
  stream?: string;
}

/** AI translate file request */
export interface AITranslateFileRequest {
  project_id: string;
  item_name: string;
  target_locale: string;
  provider: string;
  api_key: string;
  model: string;
  provider_config_id?: string;
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

// ---------------------------------------------------------------------------
// Translation Dashboard types
// ---------------------------------------------------------------------------

/** Per-locale translation progress */
export interface LocaleTranslationStats {
  locale: string;
  translated_blocks: number;
  total_blocks: number;
  translated_words: number;
  total_words: number;
  percentage: number;
}

/** Per-file translation progress */
export interface ItemTranslationStats {
  item_name: string;
  item_id: string;
  format: string;
  collection_id: string;
  block_count: number;
  word_count: number;
  locales: LocaleTranslationStats[];
}

/** Per-collection translation progress */
export interface CollectionTranslationStats {
  collection_id: string;
  collection_name: string;
  item_count: number;
  block_count: number;
  word_count: number;
  locales: LocaleTranslationStats[];
}

/** Aggregated translation dashboard statistics for a project */
export interface TranslationDashboardStats {
  locale_stats: LocaleTranslationStats[];
  item_stats: ItemTranslationStats[];
  collection_stats: CollectionTranslationStats[];
  total_blocks: number;
  translatable_blocks: number;
  total_source_words: number;
}

// ---------------------------------------------------------------------------
// Provider types
// ---------------------------------------------------------------------------

/** Saved AI provider configuration */
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

// ---------------------------------------------------------------------------
// Translation Memory types
// ---------------------------------------------------------------------------

/** TM entry info */
export interface TMEntryInfo {
  id: string;
  source: string;
  target: string;
  source_language: string;
  target_language: string;
  project_id?: string;
  stream?: string;
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

/** TM match for a single block */
export interface TMMatchInfo {
  source: string;
  target: string;
  score: number;
  match_type: string;
  project_id?: string;
  stream?: string;
}

// ---------------------------------------------------------------------------
// Terminology types
// ---------------------------------------------------------------------------

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
  project_id?: string;
  stream?: string;
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

/** Term match for a block */
export interface BlockTermMatch {
  source_term: string;
  target_terms: string[];
  domain: string;
  status: string;
  start: number;
  end: number;
  project_id?: string;
  stream?: string;
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

// ---------------------------------------------------------------------------
// Block note types
// ---------------------------------------------------------------------------

/** A user note attached to a block */
export interface BlockNote {
  id: string;
  blockId: string;
  author: string;
  text: string;
  createdAt: string;
}

// ---------------------------------------------------------------------------
// QA types
// ---------------------------------------------------------------------------

/** A single block history entry */
export interface BlockHistoryEntry {
  seq: number;
  change_type: string;
  text: string;
  coded_text: string;
  origin: string;
  author: string;
  timestamp: string;
}

/** A single QA check finding */
export interface QAIssue {
  type: string;
  severity: "error" | "warning";
  message: string;
}

/** QA results for a single block within a file */
export interface FileQAResult {
  blockId: string;
  issues: QAIssue[];
}

// ---------------------------------------------------------------------------
// Format & Tool types
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Automation types
// ---------------------------------------------------------------------------

/** Automation rule condition */
export interface AutomationCondition {
  Field: string;
  Operator: string;
  Value: string;
}

/** Automation rule action */
export interface AutomationAction {
  Type: string;
  Config: Record<string, string>;
}

/** Automation rule */
export interface AutomationRule {
  id: string;
  project_id: string;
  name: string;
  trigger: string;
  conditions: AutomationCondition[];
  actions: AutomationAction[];
  enabled: boolean;
  builtin: boolean;
  created_at: string;
  updated_at: string;
}

/** Automation event descriptor */
export interface AutomationEvent {
  type: string;
  description: string;
}

/** Save automation rule request (create or update) */
export interface SaveAutomationRuleRequest {
  name: string;
  trigger: string;
  conditions: AutomationCondition[];
  actions: AutomationAction[];
  enabled: boolean;
}

/** Automation execution history entry */
export interface AutomationHistoryEntry {
  id: string;
  rule_id: string;
  project_id: string;
  event_id: string;
  status: "success" | "failed" | "skipped";
  error: string;
  started_at: string;
  ended_at: string;
}

// ---------------------------------------------------------------------------
// Flow types
// ---------------------------------------------------------------------------

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
