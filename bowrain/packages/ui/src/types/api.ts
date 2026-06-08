/** User info from auth system */
export interface User {
  id: string;
  email: string;
  name: string;
  avatar_url: string;
  /** Set once the user has completed onboarding. Null/undefined = needs /welcome. */
  onboarded_at?: string | null;
}

/** Response from GET /api/v1/auth/me/onboarding. */
export interface OnboardingStatus {
  needs_onboarding: boolean;
  /** Email-derived suggestion to pre-fill the slug input. */
  suggested_slug?: string;
  email: string;
  display_name?: string;
}

/** Response from GET /api/v1/auth/check-slug. */
export interface SlugCheckResponse {
  available: boolean;
  /** "invalid" | "reserved" | "taken" when not available. */
  reason?: string;
}

/** Response from POST /api/v1/auth/me/email (verification email sent). */
export interface EmailChangeRequestResponse {
  status: string;
  new_email: string;
  expires_at: string;
}

/** Response from POST /api/v1/auth/email/confirm. */
export interface EmailChangeConfirmResponse {
  status: string;
  new_email: string;
}

/** Active slug rename reservation (admin view). */
export interface SlugReservation {
  slug: string;
  workspace_id: string;
  reserved_until: string;
  created_at: string;
}

/** Controls public access to the Pulse activity dashboard */
export type DashboardVisibility = "private" | "unlisted" | "public";

/** Workspace — top-level organizational unit */
export interface Workspace {
  id: string;
  name: string;
  slug: string;
  description: string;
  logo_url: string;
  type: "personal" | "team";
  languages?: string[];
  dashboard_visibility?: DashboardVisibility;
  pulse_access_key?: string;
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
  locked?: boolean;
  locked_by?: string;
  locked_at?: string;
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

/** Stream tag kind */
export type StreamTagKind = "merge" | "release" | "milestone" | "custom";

/** Stream tag — immutable marker pinned to a point in a stream's change log */
export interface StreamTag {
  id: string;
  project_id: string;
  stream: string;
  name: string;
  kind: StreamTagKind;
  cursor: number;
  metadata?: Record<string, string>;
  created_by: string;
  created_at: string;
}

/** Create stream tag request */
export interface CreateStreamTagRequest {
  name: string;
  kind?: StreamTagKind;
  metadata?: Record<string, string>;
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
  default_stream?: string;
  dashboard_visibility?: string;
  properties?: Record<string, string>;
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
  chain_key: string;
  project_id: string;
  workspace_id: string;
  event_type: string;
  actor: string;
  source: string;
  resource_type?: string;
  resource_id?: string;
  effect?: string; // "allow" | "deny" for authorization decisions
  data: string; // JSON string
  before?: string; // JSON string (prior state)
  after?: string; // JSON string (new state)
  request_id?: string;
  ip?: string;
  user_agent?: string;
  causation_id?: string;
  prev_hash?: string;
  hash?: string;
  created_at: string;
}

/** Audit log query parameters */
export interface AuditQuery {
  project?: string;
  type?: string;
  actor?: string;
  resource_type?: string;
  effect?: string;
  search?: string;
  limit?: number;
  offset?: number;
}

/** Block workflow status (ABAC). */
export type BlockWorkflowStatus = "draft" | "in_review" | "published";

/** Separation-of-duties mode for a workspace. */
export type SoDMode = "off" | "warn" | "block";

/** A team/group within a workspace. */
export interface Group {
  id: string;
  workspace_id: string;
  name: string;
  description: string;
  created_at: string;
  member_count?: number;
}

/** Binds a group to a project role. */
export interface GroupRoleBinding {
  id: string;
  group_id: string;
  workspace_id: string;
  project_id: string;
  role_id: string;
  languages: string[];
  created_at: string;
}

/** A negative-permission rule. */
export interface DenyRule {
  id: string;
  workspace_id: string;
  subject_type: "user" | "role" | "group";
  subject_id: string;
  project_id: string;
  denied_perms: number;
  reason: string;
  created_at: string;
}

/** Input for creating a deny rule. */
export interface DenyRuleInput {
  subject_type: "user" | "role" | "group";
  subject_id: string;
  project_id?: string;
  permissions: string[];
  reason?: string;
}

/** Options for restoring a stream to a past point. */
export interface RestorePointOptions {
  to_version?: string;
  to_cursor?: number;
  to_time?: string;
  stream?: string;
}

/** Result of verifying the tamper-evidence of an audit chain */
export interface AuditChainVerification {
  chain_key: string;
  rows: number;
  valid: boolean;
  broken_at?: number;
  broken_msg?: string;
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
  category?: string;
  group_key?: string;
  actor_id?: string;
  actor_name?: string;
  task_id?: string;
  priority?: string;
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
  display_name?: string;
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
  changeType: string;
  text: string;
  codedText: string;
  origin: string;
  author: string;
  actorRole?: string;
  editReason?: string;
  correlationId?: string;
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
/** One entry of a tool's IO contract (mirrors core/schema.IOPort). */
export interface IOPort {
  type: string;
  side?: "source" | "target";
  optional?: boolean;
  layer?: string;
}

export interface ToolInfo {
  name: string;
  description: string;
  category: string;
  display_name?: string;
  source?: string;
  /** Ports the tool reads upstream (non-optional = a requirement). */
  consumes?: IOPort[];
  /** Ports the tool writes. */
  produces?: IOPort[];
  /** Whether the tool may rewrite source (sit in a flow's source-transform stage). */
  is_source_transform?: boolean;
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
// Automation Runs (Bowrain AD-013)
// ---------------------------------------------------------------------------

export type RunStatus = "pending" | "running" | "completed" | "failed" | "partial";
export type StepStatus = "pending" | "running" | "completed" | "failed" | "skipped";

/** Automation run — groups all actions triggered by one event */
export interface AutomationRun {
  id: string;
  project_id: string;
  trigger_type: string;
  trigger_id: string;
  trigger_data: Record<string, string>;
  status: RunStatus;
  step_count: number;
  done_count: number;
  error?: string;
  started_at: string;
  ended_at?: string;
}

/** Automation step — a single action within a run */
export interface AutomationStep {
  id: string;
  run_id: string;
  rule_name: string;
  action_type: string;
  status: StepStatus;
  config?: Record<string, string>;
  job_ids?: string[];
  task_ids?: string[];
  total_jobs: number;
  done_jobs: number;
  error?: string;
  started_at: string;
  ended_at?: string;
}

/** Structured log entry for a step */
export interface AutomationLogEntry {
  id: string;
  step_id: string;
  run_id: string;
  level: "info" | "warn" | "error";
  message: string;
  data?: Record<string, string>;
  timestamp: string;
}

// ---------------------------------------------------------------------------
// Activity & Task types (Bowrain AD-014)
// ---------------------------------------------------------------------------

/** Activity feed entry */
export interface ActivityInfo {
  id: string;
  workspace_id: string;
  project_id?: string;
  stream?: string;
  actor_id: string;
  actor_name: string;
  type: string;
  entity_type?: string;
  entity_id?: string;
  summary: string;
  data?: Record<string, string>;
  created_at: string;
}

/** Task type discriminator */
export type TaskType =
  | "translate"
  | "review"
  | "review_terms"
  | "fix_quality"
  | "fix_brand_voice"
  | "fix_terminology"
  | "connector_setup"
  | "source_review"
  | "custom";

/** Task status */
export type TaskStatus = "open" | "in_progress" | "completed" | "cancelled";

/** Task priority */
export type TaskPriority = "low" | "normal" | "high" | "urgent";

/** Task info */
export interface TaskInfo {
  id: string;
  workspace_id: string;
  project_id: string;
  stream?: string;
  type: TaskType;
  status: TaskStatus;
  priority: TaskPriority;
  title: string;
  description?: string;
  assignee_id?: string;
  created_by: string;
  completed_by?: string;
  data?: Record<string, string>;
  due_at?: string;
  created_at: string;
  updated_at: string;
  completed_at?: string;
}

/** Create task request */
export interface CreateTaskRequest {
  project_id: string;
  stream?: string;
  type: TaskType;
  priority?: TaskPriority;
  title: string;
  description?: string;
  assignee_id?: string;
  data?: Record<string, string>;
  due_at?: string;
}

/** Notification preference per category */
export interface NotificationPreference {
  category: string;
  channels: {
    web: boolean;
    email: boolean;
    push: boolean;
    desktop: boolean;
  };
}

/** Digest email settings for a user in a workspace. */
export interface DigestSettingsDTO {
  frequency: "daily" | "weekly" | "off";
  quiet_start: string;
  quiet_end: string;
  timezone: string;
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
  /** Pipeline stage: "" or omitted = main stage; "source-transform" runs ahead. */
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
  source: string;
  created_at?: string;
  modified_at?: string;
}

// ---------------------------------------------------------------------------
// @bravo Agent (Bowrain AD-016)
// ---------------------------------------------------------------------------

/** Bravo conversation */
export interface BravoConversation {
  id: string;
  workspace_id: string;
  user_id: string;
  project_id: string;
  title: string;
  status: "active" | "completed" | "failed";
  created_at: string;
  updated_at: string;
}

/** Bravo message */
export interface BravoMessage {
  id: string;
  conversation_id: string;
  role: "user" | "assistant" | "system" | "tool";
  content: string;
  tool_calls?: BravoToolCall[];
  input_tokens?: number;
  output_tokens?: number;
  created_at: string;
}

/** Bravo tool call */
export interface BravoToolCall {
  id: string;
  message_id: string;
  tool_name: string;
  input: Record<string, unknown>;
  output?: Record<string, unknown>;
  status: "pending" | "running" | "completed" | "failed" | "needs_approval" | "denied";
  duration: number;
  error?: string;
}

/** Bravo agent config (per-workspace) */
export interface BravoConfig {
  workspace_id: string;
  enabled: boolean;
  allowed_tools?: string[];
  denied_tools?: string[];
  require_approval?: string[];
  code_exec_enabled: boolean;
  max_concurrent: number;
}

/** Bravo tool info */
export interface BravoToolInfo {
  name: string;
  require_approval: boolean;
}

/** Bravo usage summary */
export interface BravoUsageSummary {
  workspace_id: string;
  total_input_tokens: number;
  total_output_tokens: number;
  total_container_sec: number;
  message_count: number;
}

/** SSE event types for bravo streaming */
export type BravoSSEEventType =
  | "message_start"
  | "content_delta"
  | "tool_call_start"
  | "tool_call_end"
  | "needs_approval"
  | "message_end"
  | "error"
  | "step_up";

/** SSE event data: message_start */
export interface BravoSSEMessageStart {
  id: string;
  role: string;
}

/** SSE event data: content_delta */
export interface BravoSSEContentDelta {
  delta: string;
}

/** SSE event data: tool_call_start */
export interface BravoSSEToolCallStart {
  id: string;
  tool: string;
  input: Record<string, unknown>;
}

/** SSE event data: tool_call_end */
export interface BravoSSEToolCallEnd {
  id: string;
  status: string;
  output?: Record<string, unknown>;
  duration_ms: number;
}

/** SSE event data: needs_approval */
export interface BravoSSENeedsApproval {
  id: string;
  tool: string;
  input: Record<string, unknown>;
}

/** SSE event data: message_end */
export interface BravoSSEMessageEnd {
  id: string;
  usage?: {
    input_tokens: number;
    output_tokens: number;
  };
}

/** SSE event data: error */
export interface BravoSSEError {
  error: string;
}

/** SSE event data: step_up (mode restriction) */
export interface BravoSSEStepUp {
  current_mode: string;
  required_mode: string;
  action: string;
  permissions: string[];
}

/** Union of all SSE event data types */
export type BravoSSEEventData =
  | BravoSSEMessageStart
  | BravoSSEContentDelta
  | BravoSSEToolCallStart
  | BravoSSEToolCallEnd
  | BravoSSENeedsApproval
  | BravoSSEMessageEnd
  | BravoSSEError
  | BravoSSEStepUp;

/** Callback handler for SSE events from @bravo. */
export interface BravoSSEHandler {
  onMessageStart?: (data: BravoSSEMessageStart) => void;
  onContentDelta?: (data: BravoSSEContentDelta) => void;
  onToolCallStart?: (data: BravoSSEToolCallStart) => void;
  onToolCallEnd?: (data: BravoSSEToolCallEnd) => void;
  onNeedsApproval?: (data: BravoSSENeedsApproval) => void;
  onMessageEnd?: (data: BravoSSEMessageEnd) => void;
  onError?: (data: BravoSSEError) => void;
  onStepUp?: (data: BravoSSEStepUp) => void;
}

// ---------------------------------------------------------------------------
// Billing types (Bowrain AD-018)
// ---------------------------------------------------------------------------

/** Billing plan tier */
export type BillingPlan = "free" | "pro" | "team" | "enterprise";

/** Billing subscription status */
export type BillingStatus = "active" | "past_due" | "canceled" | "trialing";

/** Workspace subscription state */
export interface BillingSubscription {
  plan: BillingPlan;
  status: BillingStatus;
  seatCount: number;
  currentPeriodStart?: string;
  currentPeriodEnd?: string;
  cancelAt?: string;
}

/** Weekly credit allocation and tracking */
export interface CreditAllocation {
  creditsTotal: number;
  creditsUsed: number;
  weekStart: string;
  weekEnd: string;
  source: string;
}

/** Combined billing overview for a workspace */
export interface BillingOverview {
  subscription: BillingSubscription;
  credits: CreditAllocation;
  stripeCustomerId?: string;
}

/** Credit ledger entry (immutable transaction record) */
export interface CreditLedgerEntry {
  id: string;
  amount: number;
  balanceAfter: number;
  operation: string;
  referenceId?: string;
  createdAt: string;
}

/** Usage breakdown by operation type */
export interface BillingUsageBreakdown {
  aiTranslation: number;
  aiQualityCheck: number;
  bravoMessages: number;
  bravoContainer: number;
  total: number;
}

/** Token usage for a specific model and operation */
export interface ModelUsage {
  model: string;
  operation: string;
  prompt_tokens: number;
  output_tokens: number;
  total_tokens: number;
  call_count: number;
}

/** Runner/container time usage for a specific operation */
export interface RunnerUsage {
  operation: string;
  total_seconds: number;
  count: number;
}

/** Response from the /billing/model-usage endpoint */
export interface ModelUsageResponse {
  model_usage: ModelUsage[];
  runner_usage?: RunnerUsage[];
  from: string;
  to: string;
}

// ---------------------------------------------------------------------------
// Role Templates & Project Membership
// ---------------------------------------------------------------------------

/** Permission name (matches Go bitmask names) */
export type PermissionName =
  | "view_content"
  | "edit_source"
  | "translate"
  | "review"
  | "manage_terms"
  | "manage_tm"
  | "run_flows"
  | "manage_files"
  | "manage_streams"
  | "manage_connectors"
  | "manage_automation"
  | "manage_members"
  | "manage_project"
  | "manage_brand"
  | "manage_assets";

/** All available permissions in display order */
export const ALL_PERMISSIONS: readonly PermissionName[] = [
  "view_content",
  "edit_source",
  "translate",
  "review",
  "manage_terms",
  "manage_tm",
  "run_flows",
  "manage_files",
  "manage_streams",
  "manage_connectors",
  "manage_automation",
  "manage_members",
  "manage_project",
  "manage_brand",
  "manage_assets",
] as const;

/** Human-readable labels for permission names */
export const PERMISSION_LABELS: Record<PermissionName, string> = {
  view_content: "View content",
  edit_source: "Edit source",
  translate: "Translate",
  review: "Review",
  manage_terms: "Manage terminology",
  manage_tm: "Manage TM",
  run_flows: "Run flows",
  manage_files: "Manage files",
  manage_streams: "Manage streams",
  manage_connectors: "Manage connectors",
  manage_automation: "Manage automation",
  manage_members: "Manage members",
  manage_project: "Manage project",
  manage_brand: "Manage brand voice",
  manage_assets: "Manage assets",
};

/** Workspace-scoped role template */
export interface RoleTemplate {
  id: string;
  workspace_id: string;
  name: string;
  display_name: string;
  description: string;
  permissions: number;
  permission_names: PermissionName[];
  is_builtin: boolean;
  position: number;
  created_at: string;
  updated_at: string;
}

/** Project membership — links a user to a project with a role and language scope */
export interface ProjectMembership {
  project_id: string;
  user_id: string;
  role_id: string;
  workspace_id: string;
  languages: string[];
  created_at: string;
  user?: User;
  role_template?: RoleTemplate;
}
