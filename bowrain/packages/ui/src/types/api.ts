// IOPort is defined once in the shared @neokapi/contract-types package (#817).
import type { IOPort, Run, Anchor } from "@neokapi/contract-types";
import type { VoiceFinding, VoiceProfile } from "../voice/types";
export type { IOPort };

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

/**
 * Response from GET /api/v1/account/security — how self-service credential
 * management is surfaced for the configured identity provider.
 */
export interface AccountSecurity {
  /** True when passkeys are managed in-app (server-relayed WebAuthn ceremony). */
  in_app: boolean;
  /** External account-console URL when in_app is false (e.g. Keycloak). */
  account_url: string;
}

/** A registered WebAuthn credential (passkey). */
export interface Passkey {
  id: string;
  name: string;
  created_at?: string;
  transports?: string[];
  rp_id?: string;
}

/** Response from GET /api/v1/account/passkeys. */
export interface PasskeyListResponse {
  passkeys: Passkey[];
}

/**
 * Response from POST /api/v1/account/passkeys/register/start. `options` is the
 * opaque PublicKeyCredentialCreationOptions (JSON, base64url binary fields) to
 * hand to navigator.credentials.create(); `nonce` binds the finish call.
 */
export interface PasskeyRegisterStartResponse {
  options: unknown;
  nonce: string;
}

/** Body for POST /api/v1/account/passkeys/register/finish. */
export interface PasskeyRegisterFinishRequest {
  nonce: string;
  /** Opaque RegistrationResponseJSON from the browser's attestation. */
  attestation: unknown;
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
  /**
   * Per-workspace platform AI model. Empty/absent means the platform default is
   * used. Honored only when the platform admin has enabled customer model choice
   * and the model is in the enabled set (see PublicPlatformConfig).
   */
  preferred_model?: string;
  role: string; // current user's role in the workspace
  /**
   * Resolved feature entitlements for the current caller, keyed by feature id
   * (e.g. `bravo`). Populated by the server on the workspace GET response,
   * honoring the plan matrix + per-workspace overrides. Used to gate UI on the
   * same source of truth the server enforces. Absent in single-tenant/local mode.
   */
  features?: Record<string, boolean>;
}

/** Membership — ties a user to a workspace */
export interface Membership {
  user_id: string;
  workspace_id: string;
  role: "owner" | "admin" | "member" | "viewer";
  // Optional: a membership can reference a user record that is missing or not
  // populated (a former member, a system actor). Consumers resolve via user_id
  // when absent (see buildNameResolver).
  user?: User;
}

/** API token (stored; no plaintext) */
export interface ApiToken {
  id: string;
  user_id: string;
  workspace_id: string;
  name: string;
  /**
   * The machine this token is for, when it is a machine token. Work created
   * under it is authored by "agent/<agent_name>" rather than by user_id.
   * Absent on an ordinary personal token.
   */
  agent_name?: string;
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
  agent_name?: string;
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

/**
 * A redeemed invite: the workspace joined and the role now held. The workspace
 * fields are what the confirmation screen names and what the client switches
 * to — the accepting user has no other handle on a workspace they have only
 * just gained access to. `status` is the original shape's field, kept so a
 * caller that only checked it still can.
 */
export interface AcceptInviteResponse {
  status?: string;
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
  /** Extensible metadata, e.g. the stream-level voice binding under `voice_profile_id`. */
  properties?: Record<string, string>;
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
  /** Extensible metadata, e.g. `voice_profile_id` for the stream-level voice binding. */
  properties?: Record<string, string>;
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

/**
 * A collection's source of truth: "connected" (content synced from a connector,
 * so the source is read-only) or "managed" (UI-native — uploads/edits/deletes
 * allowed). The server folds the project-level source-connector signal into
 * this, so a collection in a connector-sourced project is "connected" even when
 * its own kind is "uploaded".
 */
export type CollectionOrigin = "connected" | "managed";

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
  /** connector_config keys whose values were redacted server-side (secrets). */
  connector_secret_keys?: string[];
  /** Source-of-truth classification (see CollectionOrigin). */
  origin?: CollectionOrigin;
  /** Whether the UI may mutate this collection's source (upload/add/delete). */
  editable?: boolean;
  /**
   * The collection's point in context space: axis → value, as the recipe
   * declares it (`product`, `channel`). Absent for a collection that sits at no
   * declared point.
   */
  coordinates?: Record<string, string>;
  /**
   * Where this collection's strings can be read in place: the component
   * explorer or running site the recipe declares. Absent when it declares none,
   * which is how the reviewer decides to offer document reading only.
   *
   * `kind` says how a view is FOUND within that host — a Storybook resolves an
   * item to a story through its published index — so a kind this client cannot
   * resolve offers no in-context reading rather than a guess.
   */
  preview?: CollectionPreview;
  item_count: number;
  created_at: string;
  updated_at: string;
}

/** A place a collection's strings can be seen in situ. */
export interface CollectionPreview {
  kind: string;
  url: string;
}

/** The preview kinds this client knows how to resolve a view within. */
export const PREVIEW_KIND_STORYBOOK = "storybook";

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

/**
 * Project type: which side owns the project's source content.
 *   - connected: content is connector-sourced (kapi push / GitHub App / git) —
 *     the source is read-only in Bowrain; review and configuration still apply.
 *   - managed: UI-native — Bowrain owns the source (uploads/edits/deletes ok).
 *   - hybrid: a mix of connected and managed collections.
 */
export type ProjectType = "connected" | "managed" | "hybrid";

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
  /**
   * Embedded per-item rows. The workspace projects LIST returns a summary
   * shape with this empty — cards and stats must read the aggregate fields
   * below. The single-project (detail) response still embeds the full array.
   */
  items?: ProjectItem[];
  collections?: CollectionInfo[];
  streams?: StreamInfo[];
  active_stream?: string;
  /**
   * Project-type rollup over the collections' origins (detail response only):
   * "connected" (all connector-sourced — read-only source), "managed"
   * (UI-native — editable), or "hybrid" (a mix). Absent on the summary list.
   */
  type?: ProjectType;
  /** Whether the UI may mutate the project's source (detail response only). */
  editable?: boolean;
  /** Server-computed aggregates (summary + detail): total files in the project. */
  item_count?: number;
  /** Server-computed aggregates: total blocks (incl. non-translatable). */
  block_count?: number;
  /** Server-computed aggregates: translatable source words. */
  word_count?: number;
  /** Server-computed aggregates: number of streams. */
  stream_count?: number;
  created_at: string;
  modified_at: string;
}

/**
 * How much of a project to read.
 *
 * `"summary"` asks for the project without its embedded item array: the name,
 * locales, properties, collections, streams and aggregates. Embedding the items
 * costs the server a block read per item, so a surface that needs none of them
 * asks for the summary and lets the dashboard endpoint serve the item list,
 * paged. `"full"` (the default) is the detail shape with `items` populated.
 */
export interface ProjectReadOptions {
  view?: "summary" | "full";
}

/** A file the server declined to import during an upload, and why. */
export interface SkippedFile {
  name: string;
  reason: string;
}

/**
 * Response of the upload endpoints: the refreshed project plus the files that
 * were not imported (undetected format, unreadable, …). A missing/empty
 * `skipped` means every file was imported.
 */
export interface UploadFilesResult extends ProjectInfo {
  skipped?: SkippedFile[];
}

/** Item within a project */
export interface ProjectItem {
  id: string;
  name: string;
  format: string;
  /**
   * The file this item's content was lifted out of, when the item is a
   * generated catalog rather than the source itself. What a list SHOWS;
   * `name` stays what everything ADDRESSES. Absent for an item that is its own
   * source, which is most of them.
   */
  source_path?: string;
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
// Access control, not progress: who may edit, while the review ladder lives on
// the per-locale target. The server normalizes the retired draft/in_review.
export type BlockWorkflowStatus = "open" | "restricted" | "published";

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

/**
 * CallerPermissions is what the signed-in user may do on one project, as the
 * server resolved it: the permission names, and the languages the
 * language-scoped ones are bound to (empty means every language).
 */
export interface CallerPermissions {
  permissions: string[];
  languages: string[];
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
  /**
   * Deployment-level capabilities the app gates UI on. `context_scan` is true
   * only when the server runs the brand-scan job system (PostgreSQL store +
   * queue); without it the hosted-scan entry points are hidden.
   */
  features?: { context_scan?: boolean };
  /**
   * AI providers a workspace admin can configure with credentials, sourced from
   * the framework provider registry (GET /api/v1/info) so the settings UI never
   * re-declares the Go provider constants.
   */
  provider_types?: ProviderTypeInfo[];
}

/** A configurable AI provider (from the framework provider registry). */
export interface ProviderTypeInfo {
  name: string;
  label: string;
  /** false for local/keyless providers (e.g. Ollama) — hides the API-key field. */
  needs_key: boolean;
}

/** A platform AI model the admin has onboarded (public-safe subset). */
export interface PlatformModel {
  id: string;
  name: string;
  tier?: string;
  requires_marketplace?: boolean;
}

/** Any active maintenance banner. */
export interface PlatformMaintenance {
  enabled: boolean;
  message?: string;
}

/**
 * Public, unauthenticated platform configuration (GET /api/v1/config). The web
 * app reads this on bootstrap for the signup gate, maintenance banner, and — when
 * `ai_customer_choice` is enabled — the enabled model set for the per-workspace
 * model picker.
 */
export interface PublicPlatformConfig {
  signups_open: boolean;
  maintenance: PlatformMaintenance;
  /** Whether workspaces may pick their own model from the enabled set. */
  ai_customer_choice: boolean;
  /** Enabled models, present only when ai_customer_choice is true. */
  ai_enabled_models?: PlatformModel[];
  /** The platform default model id (shown as the "Platform default" choice). */
  ai_default_model?: string;
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

/**
 * Per-locale lifecycle status of a committed translation — the framework's
 * `model.Target.Status` ladder ("" = no committed status yet).
 */
export type TargetStatus = "" | "draft" | "translated" | "reviewed" | "signed-off";

/**
 * The rung a clearing review call (`reviewed: false`) demotes a target to:
 * "translated" for a plain un-review (the default), "draft" for a reviewer
 * REJECTION — the unit re-enters the work queue, matching the host review
 * service's rejected → draft mapping.
 */
export type ReviewDemotion = "translated" | "draft";

/**
 * Request body for the bulk "Approve all passing" review action
 * (`POST /:ws/:id/review/approve-passing`). Both fields are optional: an
 * omitted `stream` uses the project's default stream, and omitted `locales`
 * approves across every target locale (each intersected with the caller's
 * language permission server-side).
 */
export interface ApprovePassingRequest {
  stream?: string;
  locales?: string[];
}

/**
 * Result of a bulk approve-passing call. `review_completed` is the UI's
 * "all approved · delivering…" signal: true iff the call emptied the project's
 * open review queue, so the server kicked off the completing convergence run
 * and delivery.
 */
export interface ApprovePassingResult {
  /** Blocks promoted to reviewed. */
  approved: number;
  /** Pending blocks left untouched (failing checks / non-compliant). */
  skipped: number;
  /**
   * Which bar each skipped block missed. A block missing more than one is
   * counted against the first the server applies (checks, then terminology,
   * then voice), so these sum to `skipped`. The first three are the axes the
   * queue's entries carry, so a preview and its outcome are read in one
   * vocabulary. `skipped_self_authored` is the fourth and the only one about
   * the caller: a translation they wrote, in a workspace whose
   * separation-of-duties policy blocks self-approval.
   */
  skipped_failing_checks: number;
  skipped_term_violations: number;
  skipped_below_voice_bar: number;
  skipped_self_authored: number;
  /** Pending-review targets still awaiting review after the call. */
  remaining_pending: number;
  /** True iff this call emptied the review queue → completing run + delivery. */
  review_completed: boolean;
}

/** What a back-to-source proposal asks for (RV-F). */
export type SourceProposalKind = "text-fix" | "mark-dnt";

/** Lifecycle of a back-to-source proposal. */
export type SourceProposalStatus = "open" | "approved" | "rejected";

/**
 * A reviewer's proposed change to a block's SOURCE text (back-to-source review,
 * RV-F). Any reviewer may create one while reviewing a target; a source owner
 * (PermEditSource) approves it — applying the change and re-drafting every locale
 * — or rejects it.
 */
export interface SourceProposal {
  id: string;
  workspace_id: string;
  project_id: string;
  stream?: string;
  item_name?: string;
  block_id: string;
  kind: SourceProposalKind;
  original_source: string;
  proposed_source: string;
  rationale?: string;
  /** The target locale the finder was reviewing when they caught the problem. */
  found_in_locale?: string;
  finder_user?: string;
  status: SourceProposalStatus;
  decided_by?: string;
  decision_reason?: string;
  created_at: string;
  updated_at: string;
  decided_at?: string;
}

/** Request body for `POST /:ws/:id/source-proposals`. */
export interface CreateSourceProposalRequest {
  block_id: string;
  item_name?: string;
  proposed_source: string;
  kind?: SourceProposalKind;
  rationale?: string;
  found_in_locale?: string;
  stream?: string;
}

/** Result of approving/rejecting a source proposal. */
export interface DecideSourceProposalResult {
  ok: boolean;
  status: string;
  /** True when an approval applied the source change. */
  applied?: boolean;
  /** True when the approval started a convergence run to re-draft the locales. */
  run_started?: boolean;
}

/**
 * A per-locale committed target in the blocks payload: the plain text plus its
 * lifecycle status (mirrors the server's per-locale `model.Target`). Targets
 * maps are keyed by `VariantKey.MarshalText` — for tone/channel-free variants
 * that is the plain locale string (e.g. "fr").
 */
export interface TargetInfo {
  text?: string;
  status?: TargetStatus;
}

/**
 * A `targets` map entry. New server payloads carry `TargetInfo` objects with
 * per-locale status; legacy payloads (and plain optimistic writes) carry the
 * target text as a bare string. Read through `getTargetText` /
 * `getBlockStatus` in `components/editor/blockStatus.ts` rather than indexing
 * directly.
 */
export type TargetEntry = string | TargetInfo;

/**
 * Translation block info.
 *
 * The block carries its inline content twice, and the two forms are not
 * interchangeable:
 *
 *   - `source_runs` / `targets_runs` are the RFC 0001 typed run sequences the
 *     server ships — the content model itself. Run-anchored data (overlay
 *     ranges, segment spans, check-finding positions) only means anything
 *     against these, so the run-native surfaces read them: `preview/toContentTree`
 *     projects them for the shared preview kit.
 *   - `source_coded` / `source_spans` / `targets_coded` are the flattened
 *     coded-text form the cell-level render primitives consume, derived from
 *     the runs at the read boundary (`components/editor/blockRuns`).
 *
 * Both are populated on the live path; a fixture may carry only one.
 */
export interface BlockInfo {
  id: string;
  /**
   * The id the format reader gave this block inside its item — the id the
   * rendered document carries in its `<kat-block>` markers. The store mints
   * its own `id` on ingest, so anything addressed between a block and the
   * document it came from must translate (see `documentIdOf`). Absent for a
   * block stored without an item, and for a document the server rendered from
   * the stored blocks themselves, where the two ids are already the same.
   */
  source_id?: string;
  source: string;
  /** RFC 0001 typed source runs — the server's inline-code representation. */
  source_runs?: Run[];
  source_coded?: string;
  source_spans?: SpanInfo[];
  targets: Record<string, TargetEntry>;
  /** Per-locale typed target runs, keyed like `targets`. */
  targets_runs?: Record<string, Run[]>;
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

/**
 * Ship state for one locale scope (project-wide or one collection), derived
 * server-side: `governed` = fully translated, checks pass, every translation
 * carries a human review decision; `ai_shippable` = fully translated and
 * checks pass, machine-reviewed only; `pending` = anything less.
 */
export type ShipState = "governed" | "ai_shippable" | "pending";

/**
 * Evidence behind a derived compliance rate (store.ComplianceBasis). QA checks
 * always inform it; `+terms` is added when term governance was active for the
 * scope, and `voice` when at least one block's persisted voice score, measured
 * against its profile's minimum bar, also informed it.
 */
export type ComplianceBasis = "checks" | "checks+terms" | "voice+checks" | "voice+checks+terms";

/** Per-locale translation progress */
export interface LocaleTranslationStats {
  locale: string;
  display_name?: string;
  translated_blocks: number;
  total_blocks: number;
  translated_words: number;
  total_words: number;
  percentage: number;
  /** Blocks whose translation carries a review decision (reviewed/signed-off). */
  approved_blocks?: number;
  /** Translated blocks failing QA checks with error severity (computed at full coverage). */
  failing_checks?: number;
  /** Derived ship state; absent from producers that do not derive it (e.g. pulse). */
  ship_state?: ShipState;
  /** Translated blocks counting as compliant (checks pass + voice bar where scored). */
  compliant_blocks?: number;
  /** compliant_blocks / translated_blocks in [0,1]; absent when not derived. */
  compliance_rate?: number;
  /** What informed compliance_rate; absent when the server did not derive it. */
  compliance_basis?: ComplianceBasis;
}

/** Per-file translation progress */
export interface ItemTranslationStats {
  item_name: string;
  item_id: string;
  format: string;
  /**
   * The file this item's content was lifted out of, when the item is a
   * generated catalog rather than the source itself — a KBF bundle extracted
   * from `App.tsx` is named `…/App.kbf.json`, because that path is its
   * identity. What a list SHOWS; `item_name` stays what everything ADDRESSES.
   * Absent for an item that is its own source, which is most of them.
   */
  source_path?: string;
  collection_id: string;
  /** Name of the collection collection_id refers to; absent when ungrouped. */
  collection_name?: string;
  block_count: number;
  word_count: number;
  locales: LocaleTranslationStats[];
}

/**
 * Per-collection translation progress.
 *
 * channel and coordinates project what the collection row persists, so rollups
 * can be grouped by the point in context space their content occupies. The
 * ungrouped bucket — items belonging to no collection — has an empty
 * collection_id and ungrouped: true; naming it is the consumer's call. Rollups
 * arrive ordered by collection name with the ungrouped bucket last.
 */
export interface CollectionTranslationStats {
  collection_id: string;
  collection_name: string;
  /** Delivery channel the collection is bound to; absent when unbound. */
  channel?: string;
  /** The collection's point in context space: axis → value. */
  coordinates?: Record<string, string>;
  /**
   * Where this collection's strings can be read in place, projected from the
   * collection row. Carried on the rollup because the items view opens a
   * preview from it and has no other route to the collection.
   */
  preview_kind?: string;
  preview_url?: string;
  /** True on the single bucket holding items that belong to no collection. */
  ungrouped?: boolean;
  item_count: number;
  block_count: number;
  word_count: number;
  locales: LocaleTranslationStats[];
}

/** Aggregated translation dashboard statistics for a project */
export interface TranslationDashboardStats {
  locale_stats: LocaleTranslationStats[];
  /**
   * Per-file rows. A page (server-sorted, limit/offset) when item paging
   * options were passed to getTranslationDashboard; the full list otherwise.
   */
  item_stats: ItemTranslationStats[];
  /**
   * How many items the request's scope holds, regardless of paging: the whole
   * project, or one collection when the request named one (absent from
   * pre-paging servers).
   */
  item_total?: number;
  /**
   * The directory prefix every item in the request's scope shares, with a
   * trailing slash. Computed over the whole scope, so it is stable across pages
   * and sorts; absent when the items share no prefix (or from an older server).
   * Names are shown relative to it.
   */
  item_base?: string;
  collection_stats: CollectionTranslationStats[];
  total_blocks: number;
  translatable_blocks: number;
  total_source_words: number;
}

/** Server-side sort column for the dashboard's per-file rows. */
export type DashboardItemSort = "name" | "words" | "completion";

/**
 * Item scoping/paging/sorting options for getTranslationDashboard.
 *
 * Locale totals and collection rollups are unaffected by any of these — they
 * describe the whole project on every response. Only item_stats (and, under a
 * collection scope, item_total) narrow.
 */
export interface TranslationDashboardItemOpts {
  /**
   * Scope item_stats to one collection's items, so a drill-down pages that
   * collection instead of filtering a page of the project.
   */
  itemCollection?: string;
  /**
   * Scope item_stats to the items belonging to no collection — the bucket
   * collection_stats marks with `ungrouped`. Its collection id is the empty
   * string, so no itemCollection value can name it. Mutually exclusive with
   * itemCollection.
   */
  itemUngrouped?: boolean;
  /** Max item_stats rows to return (omit for the full list). */
  itemLimit?: number;
  /** Offset into the sorted list (default 0). */
  itemOffset?: number;
  /** Server-side sort column (default "name" when paging). */
  itemSort?: DashboardItemSort;
  /** Sort direction (default "asc"). */
  itemDir?: "asc" | "desc";
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
// Content Memory types
// ---------------------------------------------------------------------------

/** content-memory entry info */
export interface MemoryEntryInfo {
  id: string;
  source: string;
  target: string;
  source_language: string;
  target_language: string;
  project_id?: string;
  stream?: string;
  updated_at: string;
}

/** content-memory search result */
export interface MemorySearchResult {
  entries: MemoryEntryInfo[];
  total_count: number;
}

/** Memory update request */
export interface MemoryUpdateRequest {
  project_id: string;
  entry_id: string;
  source: string;
  target: string;
  source_locale: string;
  target_locale: string;
}

/** One id's outcome in a bulk delete: `error` is empty exactly when `deleted`. */
export interface BulkDeleteEntryResult {
  id: string;
  deleted: boolean;
  error?: string;
}

/**
 * A bulk delete's per-id outcomes plus the totals. The call succeeds as a
 * whole even when individual ids fail, so one missing row does not strand the
 * selection. Duplicate ids are reported once. At most 500 ids per batch.
 */
export interface BulkDeleteResult {
  results: BulkDeleteEntryResult[];
  deleted: number;
  failed: number;
}

/**
 * The 409 a governed batch is refused with — deleting a concept is a change
 * the graph records, so it goes through a change-set rather than a delete.
 * `hint` names the route that opens one.
 */
export interface GovernedRefusal {
  error: string;
  detail: string;
  hint: string;
}

/** content-memory match for a single block */
export interface MemoryMatchInfo {
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
  /**
   * Optional project affinity. Omit (or leave empty) for a workspace-scoped
   * concept; the server stores an empty ProjectID for those.
   */
  project_id?: string;
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

/**
 * A single QA check finding.
 *
 * `type`, `severity` and `message` are what the Problems panel has always
 * listed. `position` is the run-anchored span the underlying
 * `core/check.Finding` carries — absent when the checker judged the whole block
 * and located nothing, which is why an unpositioned issue is recorded as a
 * block annotation rather than guessed at as a span.
 */
export interface QAIssue {
  type: string;
  severity: "error" | "warning";
  message: string;
  position?: Anchor;
  suggestion?: string;
  original_text?: string;
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
  category: string;
  display_name?: string;
  source?: string;
  tags?: string[];
  /** External resources required at runtime (target-language, tm, credentials, …). */
  requires?: string[];
  /** How many locales the tool operates on (monolingual/bilingual/multilingual). */
  cardinality?: string;
  /** Fallback second locale for bilingual tools (e.g. "qps"). */
  default_locale?: string;
  /** External systems read/written (tm-read, api-call, …). */
  side_effects?: string[];
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

/**
 * One page of automation execution history, newest first. The cursor is the
 * (started_at, id) tuple the server encodes — opaque to callers — and is
 * absent on the last page.
 */
export interface AutomationHistoryPage {
  entries: AutomationHistoryEntry[];
  next_cursor?: string;
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
// Convergence runs (server-side `kapi up`) — Bowrain AD-022
// ---------------------------------------------------------------------------

/** Lifecycle state of a server-side convergence run. */
export type ConvergenceRunState = "running" | "converged" | "parked" | "canceled" | "failed";

/** One locale's rollup within a run, mirroring the run's event stream. */
export interface ConvergenceLocaleStanding {
  locale: string;
  /** shippable | parked | pending */
  state: string;
  units?: number;
  produced?: number;
  viaTM?: number;
  viaAI?: number;
}

/**
 * A server-side convergence run — one goal-seeking reconciliation of a project
 * toward its ship gates. Matches the REST `convergenceRunView` shape from
 * bowrain/server/handlers_convergence.go.
 */
export interface ConvergenceRun {
  id: string;
  project_id: string;
  /** cli | push | manual */
  trigger: string;
  state: ConvergenceRunState;
  passes: number;
  locales?: ConvergenceLocaleStanding[];
  failing_checks?: number;
  /** Terminal cause of a failed/canceled run. */
  error?: string;
  /**
   * Machine-readable cause a run did not converge — needs_credits |
   * source_not_ready | needs_ai_key | rate_limited | no_progress |
   * checks_failing (epic 019). Empty on a converged run; drives the labeled
   * stall/hold banner and next action.
   */
  stall_reason?: string;
  /** Live loop position: settle-source | translate | recycle | … (epic 019 theme D). */
  current_stage?: string;
  current_locale?: string;
  /** Last observable progress — a frozen value while awaiting jobs reads as "waiting…". */
  last_activity?: string;
  /** How many source blocks the settle phase held below the gate ("settle first"). */
  blocked_on_source?: number;
  created_at?: string;
  finished_at?: string;
}

/**
 * The workspace's most recent convergence run within the loop rollup —
 * matches `loopRollupRunView` in bowrain/server/handlers_loop_rollup.go.
 */
export interface LoopRollupRun {
  id: string;
  project_id: string;
  project_name?: string;
  /** The project's default stream — the segment its deep links live under. */
  stream?: string;
  /** running | converged | parked | failed | canceled */
  state: string;
  /** Why a parked/failed run did not converge (needs_credits | …). */
  stall_reason?: string;
  trigger?: string;
  created_at?: string;
  finished_at?: string;
  /** The run's last observable progress. */
  updated_at?: string;
}

/** One counted project's slice of the workspace ship rollup. */
export interface LoopRollupShipProject {
  project_id: string;
  project_name?: string;
  stream?: string;
  governed: number;
  ai_shippable: number;
  pending: number;
}

/**
 * Workspace ship-state rollup: counts of project-locales per derived ship
 * state over the projects with a cached dashboard rollup (basis "cached").
 * counted_projects < total_projects means partial coverage — present it as
 * such, never as the whole workspace.
 */
export interface LoopRollupShip {
  basis: string;
  governed: number;
  ai_shippable: number;
  pending: number;
  counted_projects: number;
  total_projects: number;
  /** Counted projects, most pending first (the deep-link target leads). */
  projects?: LoopRollupShipProject[];
}

/**
 * GET /:ws/loop-rollup — the workspace home's cheap loop aggregate. Both
 * fields are optional; an absent field hides its card (no data ≠ zero).
 */
export interface LoopRollup {
  latest_run?: LoopRollupRun;
  ship?: LoopRollupShip;
}

/** The source-first readiness split of a convergence estimate. */
export interface ConvergenceSourceReadiness {
  /** Resolved gate level: none | authored | checked | approved. */
  gate: string;
  total: number;
  ready: number;
  held: number;
}

/** One locale's estimated work over the ready source. */
export interface ConvergenceEstimateLocale {
  locale: string;
  pending: number;
  via_tm: number;
  via_ai: number;
  token_estimate: number;
}

/** Cross-locale rollup of the estimate. */
export interface ConvergenceEstimateTotals {
  pending: number;
  via_tm: number;
  via_ai: number;
  token_estimate: number;
}

/** The credit/$ side of the estimate: AI cost, workspace balance, coverage. */
export interface ConvergenceEstimateCredits {
  estimated_credits: number;
  estimated_usd: number;
  balance: number;
  covers_all_ai: boolean;
  covers_ai_units: number;
}

/**
 * The provider-free pre-flight estimate for a project's next convergence run
 * (epic 019, theme B): source readiness first, then per-locale content memory/AI work for
 * the ready source, then the workspace credit balance. Matches the server's
 * convergenceEstimateView.
 */
export interface ConvergenceEstimate {
  source: ConvergenceSourceReadiness;
  locales?: ConvergenceEstimateLocale[];
  totals: ConvergenceEstimateTotals;
  credits?: ConvergenceEstimateCredits;
  note?: string;
}

/** The scope a run-now consent picks: all | ready-only | none (transport-only). */
export type ConvergenceRunScope = "all" | "ready-only" | "none";

// The convergence event protocol is the framework's (core/convergence.Event),
// shared with kapi-desktop through @neokapi/status-views — which also owns the
// one fold over it. Re-exported here so bowrain callers keep a single import
// site for their API types.
export type { ConvergenceEvent, ConvergenceEventType } from "@neokapi/status-views";

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

/**
 * One page of a workspace's activity feed (`GET /:ws/activities`), mirroring
 * the server's `ActivityListResponse`.
 *
 * `next_cursor` is absent on the last page — the server omits the key rather
 * than sending an empty string, so "there is no next page" reads the same here
 * as on every other cursor-paged answer. `new_count` is the unseen-activity
 * badge and is present only for an identified caller.
 */
export interface ActivityPage {
  activities: ActivityInfo[];
  next_cursor?: string;
  new_count?: number;
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
/** One entry of the translation review queue (GET …/pending-review/:ref). */
export interface PendingReviewEntry {
  block_id: string;
  item_name: string;
  locale: string;
  block?: BlockInfo;
  /**
   * The collection this entry's item belongs to, `""` for an item in no
   * collection. It comes from the same join the server's `collection` filter
   * tests, so a queue narrowed to a collection and a queue grouped by one
   * cannot disagree about where a row belongs.
   */
  collection_id: string;
  /**
   * This target's terminology verdict (Go store.TermCompliance). Absent or `""`
   * means no terminology governance was active for the locale — which is not
   * compliance, because nothing was checked.
   */
  term_compliance?: TermCompliance;
  /**
   * The latest persisted voice score for this block+locale, and the
   * compliance bar of the profile that produced it. Absent together for a block
   * that has never been scored; the server applies no voice bar to one either.
   */
  voice_score?: number;
  voice_bar?: number;
}

/**
 * A target's terminology verdict, as the review queue carries it. Mirror of Go
 * store.TermCompliance: three rungs, because "not checked" is not "compliant".
 */
export type TermCompliance = "" | "compliant" | "violation";

/** One page of the translation review queue plus the queue's total size. */
export interface PendingReviewPage {
  entries: PendingReviewEntry[];
  total: number;
  limit: number;
  offset: number;
}

/**
 * How one page of the review queue is scoped. Every field narrows the queue
 * server-side, so `total` describes the scope asked for — a caller paging one
 * collection is never told the project's size.
 */
export interface PendingReviewOptions {
  stream?: string;
  locales?: string[];
  limit?: number;
  offset?: number;
  /**
   * Narrow the queue to one collection's items. Undefined imposes no
   * constraint; `""` selects the items in no collection — the ungrouped bucket
   * the dashboard rollups also name — which is why the field is matched on
   * presence rather than truthiness.
   */
  collectionId?: string;
}

// ---------------------------------------------------------------------------
// Editor block queries, counts and batches (server handlers_editor.go,
// handlers_editor_bulk.go)
// ---------------------------------------------------------------------------

/** A block's per-locale progress bucket, as the server names it. */
export type BlockStatusBucket = "not-started" | "draft" | "translated" | "reviewed";

/**
 * Server-side filters for one page of a project's blocks (GET
 * …/blocks/:ref). `status` partitions the blocks for `locale` and is rejected
 * without one; `q` is a case-insensitive substring over the source and that
 * locale's target.
 */
export interface BlockQueryOptions {
  locale?: string;
  status?: BlockStatusBucket;
  q?: string;
  translatable?: boolean;
  limit?: number;
  offset?: number;
}

/** The four progress buckets, keyed as the server keys them. */
export interface BlockStatusCounts {
  "not-started": number;
  draft: number;
  translated: number;
  reviewed: number;
}

/**
 * A block query's totals and status histogram, answered without shipping a
 * block (GET …/blocks/:ref/counts). `status` partitions `translatable` for the
 * requested locale; with no locale every bucket is zero.
 */
export interface BlockCounts {
  total: number;
  translatable: number;
  locale?: string;
  status: BlockStatusCounts;
}

/**
 * One item's metadata and block tallies (GET …/items/:ref/one). Word count is
 * not part of it — …/word-count/:ref answers that.
 */
export interface ItemInfo {
  id: string;
  name: string;
  format: string;
  type: string;
  collection_id?: string;
  block_count: number;
  translatable: number;
}

/**
 * One review decision applied to a selection of blocks (POST
 * …/blocks/:ref/bulk-review). `status` picks the demotion rung when `approve`
 * is false — "translated" (the default) or "draft", a rejection that re-opens
 * the work — and is rejected alongside an approval.
 */
export interface BulkReviewBlocksRequest {
  project_id: string;
  stream?: string;
  block_ids: string[];
  target_locale: string;
  approve: boolean;
  status?: "translated" | "draft";
  comment?: string;
  item_name?: string;
}

/** One block's outcome inside a batch: `error` is empty exactly when `ok`. */
export interface BulkBlockResult {
  block_id: string;
  ok: boolean;
  status?: string;
  error?: string;
}

/**
 * The outcome of a bulk review. `review_completed` is true when the approvals
 * emptied the project's whole review queue.
 */
export interface BulkReviewBlocksResult {
  results: BulkBlockResult[];
  succeeded: number;
  failed: number;
  review_completed: boolean;
}

/**
 * The best content-memory match written into a selection of blocks (POST
 * …/blocks/:ref/bulk-apply-memory). An absent `threshold` takes the server
 * default of 1 — an exact match.
 */
export interface BulkApplyMemoryRequest {
  project_id: string;
  stream?: string;
  block_ids: string[];
  target_locale: string;
  threshold?: number;
}

/** A block that took a match, and what it took. */
export interface AppliedMemory {
  block_id: string;
  text: string;
  score: number;
}

/** A block that took nothing, and why. */
export interface SkippedMemory {
  block_id: string;
  reason: string;
}

/** The outcome of a bulk content-memory apply. */
export interface BulkApplyMemoryResult {
  applied: AppliedMemory[];
  skipped: SkippedMemory[];
}

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

/**
 * Server-side task filters. The plural forms are sets matched exactly and win
 * over their singular counterparts; `assignee_id: "me"` resolves to the caller.
 */
export interface TaskQuery {
  project_id?: string;
  assignee_id?: string;
  status?: string;
  statuses?: string[];
  type?: string;
  types?: string[];
  priority?: string;
  cursor?: string;
  limit?: number;
}

/**
 * One page of tasks (`GET /:ws/tasks`), mirroring the server's
 * `store.TaskResult`. As with every cursor-paged answer, `next_cursor` is
 * absent on the last page rather than an empty string.
 */
export interface TaskPage {
  tasks: TaskInfo[];
  next_cursor?: string;
}

/**
 * A task filter's totals, counted over the whole set rather than a page
 * (GET /:ws/tasks/counts). `by_status` is zero-filled with every known status.
 */
export interface TaskCounts {
  by_status: Record<string, number>;
  total: number;
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

/**
 * Workspace subscription state.
 *
 * These billing types are the ADAPTER's output contract, not a mirror of the
 * server's JSON: the server speaks snake_case (Go), and the REST adapter maps it
 * into these camelCase shapes. Keep the mapping in the adapter — components and
 * stories are written against these types.
 */
export interface BillingSubscription {
  plan: BillingPlan;
  status: BillingStatus;
  seatCount: number;
  currentPeriodStart?: string;
  currentPeriodEnd?: string;
  cancelAt?: string;
  /** Deadline of the local, card-free Pro trial. Absent once a subscription exists. */
  trialEndsAt?: string;
}

/**
 * Monthly plan-credit allocation and tracking. Totals describe the monthly
 * PLAN bucket only — zero for Free workspaces, whose credits are the one-time
 * trial grant surfaced via BillingOverview.spendableCredits.
 */
export interface CreditAllocation {
  creditsTotal: number;
  creditsUsed: number;
  creditsRemaining: number;
  /** When the monthly plan allowance resets (first of next month, UTC). */
  resetsAt: string;
}

/** Combined billing overview for a workspace */
export interface BillingOverview {
  subscription: BillingSubscription;
  credits: CreditAllocation;
  /**
   * Full spendable balance across every bucket: monthly plan allowance +
   * one-time trial grant + purchased packs. This is what the server's quota
   * guard actually enforces.
   */
  spendableCredits: number;
  stripeCustomerId?: string;
}

/**
 * A plan the workspace may switch to. `purchasable` is decided by the server: it
 * is false when this deployment has no Stripe price configured for the plan (a
 * self-hosted install, or production before billing is provisioned), and the UI
 * renders no upgrade button rather than one that fails.
 *
 * There are deliberately no dollar amounts here — prices live in Stripe.
 */
export interface BillingPlanInfo {
  id: BillingPlan;
  name: string;
  /** -1 means unlimited; 0 means no recurring allowance (Free). */
  monthly_credits: number;
  /** How much of the world this plan's custody covers. -1 = unlimited. */
  max_markets: number;
  /** The coarser boundary beside markets. -1 = unlimited. */
  max_brands: number;
  /**
   * How many people may author what governs content over a bounded region.
   * 0 on Free, -1 = unlimited. Every other member is free and uncapped.
   */
  max_custodians: number;
  per_seat: boolean;
  purchasable: boolean;
  current: boolean;
}

/** What this deployment can sell. */
export interface BillingPlansResponse {
  plans: BillingPlanInfo[];
  credit_pack: {
    credits: number;
    purchasable: boolean;
  };
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

/** The window, page and operation filter for the credit ledger. */
export interface CreditLedgerQuery {
  /** RFC3339; defaults to the start of the current month. */
  from?: string;
  /** RFC3339; defaults to now. */
  to?: string;
  /** Page size; the server defaults to 50 and caps at 500. */
  limit?: number;
  offset?: number;
  /** Exact operation match; absent means every operation. */
  operation?: string;
}

/**
 * One page of the credit ledger plus the totals for the whole window. Both
 * breakdowns are summed in SQL over the window, not over the page, so they stay
 * correct however small the page is.
 */
export interface CreditLedgerPage {
  entries: CreditLedgerEntry[];
  total: number;
  limit: number;
  offset: number;
  /** Spend: debits only, summed positive. What a "where did the credits go" panel shows. */
  usage_by_operation: Record<string, number>;
  /**
   * Every movement, signed — debits negative, purchases and grants positive.
   * This is the window's operation vocabulary, so it is what an operation
   * filter over the ledger reads: `usage_by_operation` omits every operation
   * that only ever adds credits, and a filter derived from it could not offer
   * the operations the ledger table itself displays.
   */
  net_by_operation: Record<string, number>;
  from: string;
  to: string;
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
  | "manage_voice"
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
  "manage_voice",
  "manage_assets",
] as const;

/** Human-readable labels for permission names */
export const PERMISSION_LABELS: Record<PermissionName, string> = {
  view_content: "View content",
  edit_source: "Edit source",
  translate: "Translate",
  review: "Review",
  manage_terms: "Manage terminology",
  manage_tm: "Manage content memory",
  run_flows: "Run flows",
  manage_files: "Manage files",
  manage_streams: "Manage streams",
  manage_connectors: "Manage connectors",
  manage_automation: "Manage automation",
  manage_members: "Manage members",
  manage_project: "Manage project",
  manage_voice: "Manage voice",
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

/**
 * Project membership — links a user to a project with a role, a language scope,
 * and a region of the project's context space.
 */
export interface ProjectMembership {
  project_id: string;
  user_id: string;
  role_id: string;
  workspace_id: string;
  languages: string[];
  /**
   * The region this membership governs — a partial point, absent meaning the
   * whole space. Combined with a role that can author what governs content, it
   * is what makes this member a custodian of that region.
   */
  coordinates?: Record<string, string>;
  created_at: string;
  user?: User;
  role_template?: RoleTemplate;
}

// ── Integration connectors (Bowrain AD-011) ─────────────────────────────────
// Workspace-scoped CMS / design / marketing integrations (WordPress, Figma,
// HubSpot). A connector is added once per workspace; fetch and publish then bind
// it to a specific project. Distinct from the PostHog locale-demand connector
// below, which is a project-scoped, read-only analytics source with its own
// dedicated routes.

/** An active connector as listed by the server (`GET /:ws/connectors`). */
export interface ConnectorInfo {
  id: string;
  /** User-supplied display name; may be empty, in which case fall back to type. */
  name: string;
  /** Connector category: "cms", "design", "marketing", … */
  category: string;
}

/**
 * A connector's sync state. Counts are best-effort — a connector reports what it
 * can compute cheaply, so some may be zero — and `errors` carries any problems
 * surfaced by the connector's last status probe. Normalised into camelCase by
 * the adapter from the server's snake_case `SyncStatus` wire shape, which the
 * desktop binding answers with too.
 */
export interface ConnectorSyncStatus {
  connectorId: string;
  /** RFC 3339 timestamp of the last sync, or "" if the connector never synced. */
  lastSync: string;
  itemCount: number;
  fileCount: number;
  wordCount: number;
  pendingPull: number;
  pendingPush: number;
  errors: string[];
}

/**
 * A whole panel's connector states in one read (`GET /:ws/connectors/status`).
 * `statuses` is keyed by the connector id asked for; `unknown` names the ids
 * neither the live service nor the config store could answer — a probe that
 * failed with no stored state to degrade to.
 */
export interface ConnectorStatusBatch {
  statuses: Record<string, ConnectorSyncStatus>;
  unknown: string[];
}

/**
 * One item a connector can see, for the read-only content browser. This is a
 * verbatim mirror of the Go `bowrain/core/connector.ContentItem` struct, which the
 * server marshals with NO json tags — so keys are PascalCase, not camelCase.
 * On a listing `Blocks` is null (List does not fetch content) and `LastChanged`
 * is an RFC3339 string. The desktop maps its ContentItemInfo binding into the
 * same shape.
 */
export interface ConnectorContentItem {
  ID: string;
  Name: string;
  Path: string;
  /** Detected format/type id (e.g. "html", "json"); "" when the source reports none. */
  Format: string;
  Locale: string;
  /** Full block payload; null on listings (List does not fetch content into the store). */
  Blocks: unknown[] | null;
  Metadata: Record<string, string> | null;
  /** RFC3339 timestamp of the item's last change. */
  LastChanged: string;
}

// ── PostHog locale-demand connector (phase 0, read-only) ────────────────────

/**
 * PostHog connector config as returned by the server. The personal API key is
 * never returned — `api_key_masked` carries "••••" + its last four characters.
 */
export interface PostHogConnectorConfig {
  configured: boolean;
  /** "us", "eu", or a self-hosted https URL. */
  host?: string;
  /** Short display label for the host, e.g. "us.posthog.com". */
  host_label?: string;
  posthog_project_id?: string;
  api_key_masked?: string;
  path_locale_pattern?: string;
  updated_at?: string;
}

/**
 * Connect/update payload. An empty `api_key` on update keeps the stored key
 * (the client never sees it, so it cannot resend it); a value rotates it.
 */
export interface PostHogConnectorConfigRequest {
  host: string;
  posthog_project_id: string;
  api_key?: string;
  path_locale_pattern?: string;
}

/** (language tag, sessions) pair inside a country breakdown. */
export interface PostHogLanguageSessions {
  tag: string;
  sessions: number;
}

/** Demand observed from one country ($geoip_country_code, alpha-2). */
export interface PostHogCountryDemand {
  country_code: string;
  sessions: number;
  languages: PostHogLanguageSessions[] | null;
}

/** One week of a language's demand trend. */
export interface PostHogTrendPoint {
  week_start: string;
  sessions: number;
}

/** One language aggregated across countries. */
export interface PostHogLanguageDemandInfo {
  tag: string;
  sessions: number;
  trend?: PostHogTrendPoint[] | null;
}

/** Where a demand snapshot came from — for the page's provenance footer. */
export interface PostHogDemandSourceInfo {
  kind: string; // "posthog"
  host_label: string;
  posthog_project_id: string;
}

/**
 * Demand snapshot from /connectors/posthog/demand. Warnings carry per-query
 * failures: the snapshot is best-effort partial data, never all-or-nothing.
 */
export interface PostHogDemandResponse {
  range: string;
  generated_at: string;
  total_sessions: number;
  countries: PostHogCountryDemand[] | null;
  languages: PostHogLanguageDemandInfo[] | null;
  served_locale_hit_rate?: number;
  warnings?: string[] | null;
  source: PostHogDemandSourceInfo;
  cached_at: string;
  cached: boolean;
}

// ---------------------------------------------------------------------------
// Context scan (AI brand onboarding — epic 016)
// ---------------------------------------------------------------------------

/**
 * Request body for POST /api/v1/{ws}/context-scans. At least one source
 * (paste_text, urls, repo_url, or upload_keys) is required.
 */
export interface ContextScanRequest {
  /** Freeform pasted text, used as-is. */
  paste_text?: string;
  /** Public https pages to fetch (max 5). */
  urls?: string[];
  /** Git repository whose docs/README are read for brand signal. */
  repo_url?: string;
  /** Blob keys returned by the brand-scan upload endpoint. */
  upload_keys?: string[];
  /** Name for the drafted profile. */
  profile_name?: string;
  /** Subject domain hint passed to the inference tool. */
  domain?: string;
}

/** One stored upload from POST /api/v1/{ws}/context-scans/uploads. */
export interface ContextScanUpload {
  key: string;
  filename: string;
  size: number;
}

/** Response of the brand-scan upload endpoint. */
export interface ContextScanUploadResult {
  uploads: ContextScanUpload[];
  /** Files that were not stored (disallowed type, oversize, deferred pdf/pptx). */
  skipped?: SkippedFile[];
}

/** Model confidence and source rationale for one inferred profile field. */
export interface ContextScanFieldEvidence {
  /** 0–1 confidence in the inference for the field. */
  confidence: number;
  /** Short note describing the corpus evidence the inference rests on. */
  source: string;
}

/** Evidence sidecar keyed by field name (tone, style, vocabulary, examples). */
export interface ContextScanEvidence {
  fields: Record<string, ContextScanFieldEvidence>;
}

/** One candidate glossary term extracted from the corpus. */
export interface ContextScanTerm {
  term: string;
  definition: string;
  domain: string;
}

/** One corpus source that contributed to the scan. */
export interface ContextScanSource {
  kind: string;
  label: string;
  runes: number;
}

/**
 * What a scan proposes. Open by intent — gates and redaction rules are
 * governance bound at a point in the same way — but a kind arrives with the
 * inference that produces it, so this is the set that exists today.
 */
export type ContextScanArtefactKind = "voice" | "terms";

/**
 * One thing a scan proposes, and the point it would govern.
 *
 * `at` is that point as an axis map, the same open shape a sync context entry
 * carries. An ABSENT or EMPTY `at` means the project's default point — whatever
 * `defaults.coordinates` resolves to — which is the onboarding case: a scan
 * that finds no structure proposes one voice for the project.
 *
 * Only the fields matching `kind` are populated. Mirrors jobs.ArtefactProposal.
 */
export interface ContextScanArtefact {
  at?: Record<string, string>;
  kind: ContextScanArtefactKind;
  /** kind === "voice" */
  voice?: VoiceProfile;
  /** kind === "voice" */
  evidence?: ContextScanEvidence;
  /** kind === "terms" */
  terms?: ContextScanTerm[];
}

/**
 * One dimension the corpus varies along, and the values it takes. The axis name
 * is the corpus' own vocabulary rather than a name from a fixed list: a project
 * has product lines, or regional markets, or audiences, and the context space is
 * whatever that project actually distinguishes.
 *
 * Mirrors tools.AxisProposal.
 */
export interface ContextScanAxis {
  axis: string;
  values: string[];
  evidence?: string[];
  confidence: number;
}

/** The reviewable output of a completed context scan. Mirrors jobs.ContextScanResult. */
export interface ContextScanDraft {
  /**
   * Absent or empty is the ordinary answer for a project whose content is
   * uniform, and for a first scan of one small site. Not a failure, and not a
   * reason to withhold the artefacts.
   */
  axes?: ContextScanAxis[];
  artefacts: ContextScanArtefact[];
  sources: ContextScanSource[];
  truncated: boolean;
}

/**
 * One axis a reviewer approved, and where it applies. Mirrors
 * server.ApproveAxisRequest.
 *
 * `collection` is required for the structural axes (`product`, `channel`) and
 * refused for the rest: those two are derived from a collection's `channel:`,
 * so approving one is a claim about a particular collection, and which one the
 * scan cannot know — it read a corpus, never the project's collections.
 */
export interface ApproveAxisRequest {
  axis: string;
  value: string;
  collection?: string;
}

/**
 * One recipe field an approval is waiting to write. Mirrors
 * store.PendingRecipeChange.
 *
 * Approving an axis does not declare it. The row waits for a pull to put the
 * line in `kapi.yaml`, where it is reviewed in git like any other change; the
 * recipe stays the only thing that mints a coordinate.
 */
export interface PendingRecipeChange {
  id: string;
  workspace_id: string;
  project_id: string;
  /** Dotted recipe path, e.g. `collections.docs.channel`. */
  path: string;
  value: unknown;
  status: "pending" | "applied";
  created_by?: string;
  created_at: string;
  applied_at?: string;
}

export type ContextScanStatus = "queued" | "processing" | "completed" | "failed";

/** State of a brand-scan job from GET /api/v1/{ws}/context-scans/{id}. */
export interface ContextScanJob {
  id: string;
  status: ContextScanStatus;
  progress: number;
  message: string;
  tokens_used: number;
  error?: string;
  /** Present only when status is "completed". */
  draft?: ContextScanDraft;
}

/** One candidate term a reviewer kept, on approval. */
export interface ContextScanApprovedTerm {
  term: string;
  definition?: string;
  domain?: string;
  /** Falls back to the request's `locale`, then to "en". */
  locale?: string;
}

/**
 * The reviewed outcome of a context scan: the edited draft profile and the
 * candidate terms that survived review, applied in one transaction.
 */
export interface ContextScanApproveRequest {
  /**
   * The point the approved artefacts would govern, as an axis map. Absent or
   * empty is the project's default point. The server refuses a point whose
   * axes the workspace declares no content on, with a 409.
   */
  at?: Record<string, string>;
  profile: VoiceProfile;
  terms?: ContextScanApprovedTerm[];
  /** Locale approved terms are created in when a term does not name its own. */
  locale?: string;
}

/**
 * What an approval applied: the stored profile and which action produced it,
 * plus the concepts created and the ones already there. Terms are created at
 * status "proposed", and only where no concept already carries them in that
 * locale — so a retry after a partial application is safe.
 */
export interface ContextScanApproveResult {
  profile: VoiceProfile;
  profile_action: "created" | "updated" | "unchanged";
  concepts_created: number;
  concepts_existing: number;
  concept_ids: string[];
}

/** Result of the stateless draft check (live tester). */
export interface ContextScanCheckResult {
  /** core/profile.VoiceComplianceScore — the roll-up plus per-dimension detail. */
  score: { overall: number; dimensions?: unknown[]; word_count?: number };
  /** core/check.Finding, the same shape the profile check and stored scores emit. */
  findings: VoiceFinding[];
}

/** The signed state that ties a GitHub App installation back to the workspace
 * that started the install. It travels to GitHub as the install URL's `state`
 * and comes back on the setup redirect. */
export interface GitHubSetupState {
  state: string;
  /** Seconds the state stays valid. */
  expires_in: number;
}

/** Result of claiming an installation for the current workspace. */
export interface ClaimInstallationResult {
  installation_id: number;
  /** The GitHub organization or user the app is installed on, when known. */
  account?: string;
}

/** One repository a GitHub App installation covers, with its binding if a
 * forge connector already tracks it. */
export interface InstallationRepo {
  full_name: string;
  default_branch: string;
  private: boolean;
  connector_id?: string;
  project_id?: string;
}

/** Request to bind an installed repository to a project. */
export interface BindInstallationRepoRequest {
  repository: string;
  project_id: string;
  branch?: string;
  patterns?: string;
  name?: string;
}

/** Result of binding an installed repository. */
export interface BindInstallationRepoResult {
  connector_id: string;
  repository: string;
  project_id: string;
  branch: string;
  /** "started" when the server kicked off the initial background ingest. */
  initial_fetch?: string;
}

/** One detected i18n/content signal in a repository's tree. */
export interface RepoContentSignal {
  /** Signal id — an i18n framework registry id (react-i18next, flutter, …) or a generic kind. */
  id: string;
  /** Human fragment for the conclusion line, e.g. "i18next catalogs". */
  label: string;
  /** Catalog root seen in the tree (absent when repo-wide). */
  dir?: string;
  /** How many tree files back this signal. */
  files: number;
}

/**
 * What the detect endpoint reports for a repository before binding: monorepo
 * shape, content signals, a proposed connector `patterns` value, and the
 * files that proposal (or a caller-supplied override) matches.
 */
export interface RepoDetection {
  monorepo_markers: string[] | null;
  workspaces?: string[] | null;
  signals: RepoContentSignal[];
  proposed_patterns: string;
  match_count: number;
  match_preview: string[];
  truncated: boolean;
}

/** Optional query knobs for the detect endpoint. */
export interface RepoDetectOptions {
  /** Confine detection to a subdirectory (monorepo workspace). */
  scope?: string;
  /** Comma-separated globs to match instead of the proposal (live feedback while editing). */
  patterns?: string;
}

// ---------------------------------------------------------------------------
// Measured steerability (model recommendation sweeps)
// ---------------------------------------------------------------------------

/**
 * One persisted sweep measurement: how a candidate model scored on the
 * project's own trap fixtures for a locale, with the project's full brand
 * context (adherence) and bare (adherence_bare); lift is the difference.
 */
export interface ModelSweepMeasurement {
  model: string;
  /** 0..1 — fixture adherence with the project's full brand context. */
  adherence: number;
  /** 0..1 — fixture adherence with no context at all. */
  adherence_bare: number;
  /** adherence − adherence_bare. */
  lift: number;
  fixture_count: number;
  tokens_used: number;
  measured_at: string;
  /** True on the row the recommendation policy picked for this locale. */
  recommended: boolean;
}

/** A locale's measurements plus the recommended model (absent when none qualifies). */
export interface ModelSweepLocaleGroup {
  locale: string;
  recommended_model?: string;
  models: ModelSweepMeasurement[];
}

/**
 * GET /:ws/:id/model-recommendations — `enabled` mirrors the instance-wide
 * model_sweeps.enabled gate so the settings panel can disable Refresh with a
 * reason.
 */
export interface ModelRecommendationsResponse {
  enabled: boolean;
  locales: ModelSweepLocaleGroup[];
}
