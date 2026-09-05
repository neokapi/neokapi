// Types matching the Go backend structs exposed via Wails bindings.

// IOPort (and the schema-language cluster, re-exported below) is defined once in
// the shared @neokapi/contract-types package (issue #817).
import type { Anchor, IOPort, ReviewContext, Run } from "@neokapi/contract-types";

export interface KapiProject {
  version: string;
  name: string;
  plugins?: Record<string, PluginSpec>;
  defaults?: ProjectDefaults;
  collections?: Collection[];
  preset?: string;
  flows?: Record<string, FlowSpec>;
  /** Governance bound to a product, keyed by the product's name. */
  profiles?: Record<string, ProfileSpec>;
}

/** A profile: the channels a product ships on, and what governs them. */
export interface ProfileSpec {
  channels?: (string | { id: string; concept?: string })[];
  voice?: VoiceBindingSpec;
  termstore?: string;
  concept?: string;
  valid_from?: string;
  valid_to?: string;
}

/** A voice binding: exactly one of a file, a starter pack, or a stored name. */
export interface VoiceBindingSpec {
  profile_file?: string;
  profile?: string;
  pack?: string;
}

export interface PluginSpec {
  version?: string;
  framework_version?: string;
  format_priority?: number;
}

export interface ProjectDefaults {
  source_language?: string;
  target_languages?: string[];
  locale_format?: string;
  concurrency?: number;
  parallel_blocks?: number;
  encoding?: string;
  formats?: Record<string, FormatDefaults>;
  /** The default flow `kapi up` / Bring up to date converges with. */
  flow?: string;
  /** Glob patterns skipped during content scanning. */
  exclude?: string[];
  /** The voice profile bound as standing project context. */
  voice?: VoiceBindingSpec;
  /**
   * The project's default point: the declared axes every collection sits at
   * unless it says otherwise. The structural axes (product, channel) are
   * derived from a collection's `channel:` and never written here.
   */
  coordinates?: Record<string, string>;
}

export interface FormatDefaults {
  preset?: string;
  config?: Record<string, unknown>;
  priority?: number;
}

/**
 * A collection is either a bare entry (has path, no content) or a named
 * collection (has name and content).
 */
export interface Collection {
  // Collection fields (long form).
  name?: string;
  source_language?: string;
  target_languages?: string[];
  content?: ContentItem[];

  /** Directory this collection lives in: a prefix joined onto every content
   * path, target and item base below it. */
  base?: string;

  /** The point in the project's context space this collection's content sits
   * at: `profile/channel`, or a bare `channel` when exactly one profile
   * declares it. */
  channel?: string;

  /** Declared axes this collection sits at, overriding the project's own. */
  coordinates?: Record<string, string>;

  // Bare entry fields (short form — promoted from ContentItem).
  path?: string;
  format?: FormatSpec;
  target?: string;

  // Optional archived-state marker; gates the Translation-state section in
  // the CollectionsPanel (absent on most collections).
  archive?: boolean;
}

export interface ContentItem {
  path: string;
  format?: FormatSpec;
  target?: string;
  /** Directory the source path is made relative to for target resolution
   * ({path}/{dir}/{relpath} tokens and directory-mirror targets). Empty
   * defaults to the glob's fixed prefix. */
  base?: string;
  source_language?: string;
  target_languages?: string[];
}

/**
 * Format spec — either a short form (just name as string from YAML) or
 * long form (name + preset + config). In TypeScript, always represented
 * as the full object since JSON from Go always sends the struct.
 */
export interface FormatSpec {
  name: string;
  preset?: string;
  config?: Record<string, unknown>;
}

export interface FlowSpec {
  /** Markdown — see markdown-in-ui.md. */
  description?: string;
  steps: FlowStep[];
}

export interface FlowStep {
  /** Empty on a parallel group, which fans out to `parallel`. */
  tool: string;
  config?: Record<string, unknown>;
  label?: string;
  parallel?: FlowStep[];
}

export interface FlowIssue {
  tool: string;
  type: "unknown" | "undeclared_plugin";
  message: string;
}

/** One retained trace of the last run: the input file and locale pass it recorded. */
export interface RunTraceFile {
  file_path: string;
  locale?: string;
  output_path?: string;
  /** The recording budget cut the trace short: it holds the run's first parts. */
  truncated?: boolean;
}

/**
 * The last run's replayable record: the flow that ran, the steps it ran with,
 * and the files whose traces the backend kept, in completion order.
 */
export interface RunTraces {
  flow_name: string;
  steps: FlowStep[];
  files: RunTraceFile[];
  /** The recording budget: a truncated trace holds this many parts. */
  max_parts: number;
}

/** One channel in the channel map: where content sits, and what governs there. */
export interface ChannelMapRow {
  /** profile/channel, the way a collection names it. */
  ref: string;
  profile: string;
  channel: string;
  coordinates?: Record<string, string>;
  /** True when a profile declares the channel (so it can be renamed). */
  declared: boolean;
  /** The governing voice profile's name, if one binds. */
  voice?: string;
  collections: string[];
  item_count: number;
}

/** The channel map for a project. */
export interface ChannelMapResult {
  channels: ChannelMapRow[];
  notes?: string[];
}

export interface FlowInfo {
  name: string;
  /** Markdown — see markdown-in-ui.md. */
  description: string;
  step_count: number;
  /** Each step named for the card's chip strip, in order. */
  steps?: string[];
  /** True when this flow is the project's defaults.flow. */
  default?: boolean;
  valid: boolean;
  issues?: FlowIssue[];
}

export type LocaleCardinality = "monolingual" | "bilingual" | "multilingual";

export interface ToolInfo {
  name: string;
  display_name?: string;
  /** Markdown — see markdown-in-ui.md. */
  description: string;
  category: string;
  source?: string;
  has_schema: boolean;
  tags?: string[];
  requires?: string[];
  cardinality?: LocaleCardinality;
  default_locale?: string;
  /** Ports the tool reads upstream (non-optional = a requirement). */
  consumes?: IOPort[];
  /** Ports the tool writes. */
  produces?: IOPort[];
  side_effects?: string[];
  /** Whether the tool is a transformer — it may rewrite the source (AD-006). */
  is_source_transform?: boolean;
  /** A recoverable transformer (redaction) vaults originals and restores them later. */
  recoverable?: boolean;
}

export interface FormatInfo {
  name: string;
  display_name?: string;
  extensions?: string[];
  mime_types?: string[];
  has_reader: boolean;
  has_writer: boolean;
  source?: string;
  has_schema: boolean;
}

// --- Content checks (matches backend DesktopFinding / CheckFileResult / CheckRunResult) ---

/** One content-check finding, flattened for the Checks panel. */
export interface DesktopFinding {
  category: string;
  severity: string; // "neutral" | "minor" | "major" | "critical"
  message: string;
  suggestion?: string;
  original_text?: string;
  /** The format's stable block id, so a fix can re-find the block. */
  block_id?: string;
  /** Which side of the block the offending text lives on. */
  field?: "source" | "target";
  /** The language `original_text` is written in, for direction and lang attributes. */
  locale?: string;
  /** Structured fix text (e.g. a voice profile's preferred term). */
  replacement?: string;
  /** Whether the panel may show a one-click "Apply fix" button. */
  fixable: boolean;
  /** The rule that fired, so the finding traces to the decision behind it. */
  rule?: string;
  /** The coordinate the checked file sits at. Empty is the project's own point. */
  point?: string;
  /** The collection the checked file belongs to. */
  collection?: string;
  /** The run range the finding applies to, so the offending words can be
   *  underlined rather than described. Anchored to the block's source runs
   *  (core/check.Finding). Absent when the checker objects to something that
   *  is missing from the text. */
  position?: RunAnchor;
  /** The block's source runs, so the finding can be read in the text it was
   *  raised on with `position` marked over it. */
  source_runs?: Run[];
  /** The block's runs in the target locale a target-side finding names, when
   *  the block carries them. */
  target_runs?: Run[];
}

/** Where in a block's runs something sits. Mirrors model.Anchor. */
/**
 * Where inside a block a finding sits (model.Anchor): the whole block, one run
 * by id, a half-open span of run positions, or a plural form or select case.
 * The generated contract type, so the desktop reads the same shape the preview
 * kit resolves.
 */
export type RunAnchor = Anchor;

/** One content-verification result as the engine produces it (core/check.Finding). */
export interface CheckFinding {
  category: string;
  severity: string; // "neutral" | "minor" | "major" | "critical"
  message: string;
  suggestion?: string;
  position: RunAnchor;
  original_text?: string;
  /** The checker that produced it. */
  check?: string;
  metadata?: Record<string, string>;
}

/** Findings grouped by content file. */
export interface CheckFileResult {
  path: string;
  findings: DesktopFinding[];
}

/** Result of a RunChecks pass: pass/fail gate, roll-up score, per-file findings. */
export interface CheckRunResult {
  pass: boolean;
  score: number;
  files: CheckFileResult[];
}

export interface PluginCapability {
  type: string;
  name: string;
  display_name?: string;
  extensions?: string[];
}

export interface PluginInfo {
  name: string;
  id: string;
  version: string;
  framework_version?: string;
  /** Markdown — see markdown-in-ui.md. */
  description?: string;
  type: string;
  formats?: string[];
  capabilities?: PluginCapability[];
}

export interface PluginStatus {
  satisfied: boolean;
  issues?: PluginIssue[];
}

export interface PluginIssue {
  plugin: string;
  type: "missing" | "version_mismatch";
  required?: string;
  installed_version?: string;
}

export interface ProviderConfig {
  id: string;
  name: string;
  provider_type: string;
  model?: string;
  base_url?: string;
  /** The default credential for its provider when more than one is saved. */
  default?: boolean;
}

/**
 * Which layer supplied an effective AI provider/model (matches Go
 * host.AIModelOrigin). Ordered high to low precedence.
 */
export type AIModelOrigin = "locale-preset" | "project-preset" | "app-config" | "built-in";

/**
 * The provider/model an AI step will actually run with, and where each half came
 * from (matches Go host.AIModelResolution). Derived by the shared
 * host.EffectiveAIModel, so the desktop reports exactly what the CLI resolves.
 */
export interface AIModelResolution {
  provider: string;
  model: string;
  provider_origin: AIModelOrigin;
  model_origin: AIModelOrigin;
  /** False when nothing is configured and the built-in fallback is carrying the run. */
  configured: boolean;
}

/** The configured default AI model and the provider it implies (matches Go DefaultModelInfo). */
export interface DefaultModelInfo {
  provider: string;
  model: string;
}

/** One selectable model in the model-first "AI Models" picker (matches Go AIModelOption). */
export interface AIModelOption {
  model: string;
  provider: string;
  /** Provider display label (e.g. "Ollama"). */
  label: string;
  /** On-device provider (Ollama) — no API key needed. */
  local: boolean;
  /** Local only: already present on the Ollama server. */
  installed: boolean;
  /** Cloud model with no saved credential yet. */
  needs_key: boolean;
  /** Runs on a personal subscription via a detected keyless provider
   * (claude-code) — no API key, no metered cost. */
  subscription?: boolean;
  /** Optional one-line rationale (recommended local models). */
  note?: string;
  /** The currently configured default. */
  is_default: boolean;
}

/** One keyless AI provider found on this machine (backend DetectedAIProvider). */
export interface DetectedAIProvider {
  provider: string;
  label: string;
  model: string;
  /** One-line card subtitle (e.g. "signed in on this Mac · uses your Claude subscription"). */
  detail: string;
  /** Bills a personal subscription (claude-code) rather than metered API usage. */
  subscription: boolean;
}

/** The machine's AI options (backend AIDetectionResult) — powers the
 * first-open "Connect your AI" card and the Settings "Detected" section. */
export interface AIDetectionResult {
  detected: DetectedAIProvider[] | null;
  env_key_providers?: string[];
  saved_credential_providers?: string[];
  default_provider?: string;
  default_model?: string;
  /** True when any provider is already usable without setup. */
  configured: boolean;
}

export interface TabInfo {
  id: string;
  name: string;
  path: string;
}

/** A saved "Active Filter": narrows the project to a subset of collections
 * (optionally a glob within them) and target languages (matches Go ProjectFilter). */
export interface ProjectFilter {
  id: string;
  name: string;
  collections?: string[];
  glob?: string;
  languages?: string[];
  /** Committed to the project (.kapi/filters.json) vs personal (filters.local.json). */
  shared?: boolean;
}

/** The project's saved filters plus the active selection (matches Go ProjectFilters). */
export interface ProjectFilters {
  /** Active filter id; "" means no filter ("All"). */
  active: string;
  filters: ProjectFilter[];
}

/** A single file-dialog filter handed to BrowsePath (matches the Go BrowsePathFilter). */
export interface BrowsePathFilter {
  name: string;
  /** Space-delimited glob list, e.g. "*.tmx" or "*.html *.htm". */
  extensions: string;
}

/** Generic browse request the schema-form PathPicker hands to the host (matches Go BrowsePathRequest). */
export interface BrowsePathRequest {
  kind: "file" | "directory";
  field: string;
  currentValue?: string;
  title?: string;
  forSaveAs?: boolean;
  filters?: BrowsePathFilter[];
  accepts?: string[];
}

// Schema types — re-exported from the shared contract-types package (single
// source of truth, issue #817).
export type {
  IOPort,
  ComponentSchema,
  FormatMeta,
  ToolMeta,
  ParameterGroup,
  PropertySchema,
} from "@neokapi/contract-types";

// --- Plugin documentation types (from docs.json) ---

/** Summary returned by GetPluginDocs — lists available doc IDs. */
export interface PluginDocsSummary {
  generatedAt?: string;
  wikiBaseUrl?: string;
  filterIDs?: string[];
  stepIDs?: string[];
  aliases?: Record<string, string>;
}

/**
 * Full docs bundle used in Storybook fixtures and for pre-loaded data.
 * In the real app, individual docs are fetched via getFilterDoc/getStepDoc.
 */
export interface PluginDocs {
  generatedAt?: string;
  wikiBaseUrl?: string;
  filters: Record<string, FilterDoc>;
  steps: Record<string, StepDoc>;
  aliases?: Record<string, string>;
  concepts?: Record<string, ConceptDoc>;
}

// The *Doc `overview`, `limitations`, `processingNotes`, per-parameter
// help, and example `description` fields are markdown — render them through
// the shared `Markdown` primitive (@neokapi/ui-primitives). See
// web/docs/contribute/implementation/repo/markdown-in-ui.md.

export interface FilterDoc {
  filterName: string;
  /** Markdown. */
  overview: string;
  filterId?: string;
  wikiUrl?: string;
  parameters?: Record<string, ParameterDoc>;
  /** Markdown items. */
  limitations?: string[];
  /** Markdown items. */
  processingNotes?: string[];
  examples?: DocExample[];
}

export interface StepDoc {
  filterName: string; // actually the step display name
  /** Markdown. */
  overview: string;
  stepId?: string;
  wikiUrl?: string;
  parameters?: Record<string, ParameterDoc>;
  /** Markdown items. */
  limitations?: string[];
  /** Markdown items. */
  processingNotes?: string[];
  examples?: DocExample[];
}

export interface ParameterDoc {
  /** Markdown (parameter help). */
  description?: string;
  /** Markdown. Alias for description used in okapi-bridge doc files. */
  help?: string;
  notes?: string[];
  introducedIn?: string;
  dependsOn?: ParameterDependency[];
  values?: string;
  seeAlso?: string;
}

export interface ParameterDependency {
  property: string;
  condition: string;
}

export interface DocExample {
  title: string;
  /** Markdown. */
  description?: string;
  input?: string;
  output?: string;
}

export interface ConceptDoc {
  wikiRef?: string;
  description?: string;
  [key: string]: unknown;
}

export type AppMode = "adhoc" | "projects";

/**
 * Persisted project-first session restored on the next launch.
 * lastOpenProjects are recipe paths, most-recent first.
 */
export interface SessionState {
  mode: string;
  lastOpenProjects: string[];
  activeProject: string;
}

/** Per-collection translation status rendered on the project home. */
export interface CollectionStatus {
  name: string;
  /** Total translatable blocks across the collection's files. */
  blockCount: number;
  /** Maps locale → count of units carrying a translation, from the working tree. */
  coverage: Record<string, number>;
  /**
   * Maps locale → how many units that count is out of, from the same
   * derivation. This is the denominator a percentage uses: blockCount answers
   * a different question (how much has been extracted) and a ratio mixing the
   * two belongs to neither.
   */
  units: Record<string, number>;
  targetLanguages: string[];
}

/** Project-wide extraction + coverage status. */
export interface ProjectStatus {
  projectPath: string;
  projectName: string;
  /** false ⇒ never extracted; show the shell + a "run extract" prompt. */
  hasData: boolean;
  /** true ⇒ the block store exists but was written by a different kapi version
   *  than the running binary, so the counts may be wrong — surface a
   *  "re-extract" affordance rather than showing stale numbers as authoritative. */
  stale?: boolean;
  collections: CollectionStatus[];
}

// --- Convergence (the derived state model: cli.ConvergenceReport) ---

/** One unmet gate threshold: a state below its required percent. */
export interface GateShortfall {
  state: string;
  actual: number;
  required: number;
  /** Approver class of the unmet threshold when explicitly set (human|any). */
  by?: string;
}

/** Per-(collection, locale) target coverage + ship-gate standing. */
export interface LocaleCoverage {
  locale: string;
  collection?: string;
  total: number;
  /** Ladder state → "at least" percent (draft|translated|reviewed|signed-off). */
  pct: Record<string, number>;
  gated: boolean;
  shippable: boolean;
  pending?: GateShortfall[];
}

/** Project-wide source authoring readiness (authored|checked|approved). */
export interface SourceCoverage {
  total: number;
  pct: Record<string, number>;
  gated: boolean;
  shippable: boolean;
  pending?: GateShortfall[];
  /** Units whose reviewed rung came from an autonomous AI decision ("ai/…").
   * Shown with an "(ai)" qualifier; human-required gates do not count them. */
  aiReviewed?: number;
}

/** One unit awaiting a person: a translation not yet approved, or a source unit
 *  the project's source gate is waiting on (matches Go
 *  convergence.ReviewQueueItem). */
export interface ReviewItem {
  locale: string;
  file: string;
  key: string;
  /** The language this row belongs to: the target locale for a translation, the
   *  project's source language for a source unit. */
  language?: string;
  /** The author's own wording awaiting attention, rather than a translation. */
  isSource?: boolean;
  /** The row's rung on its own ladder: `translated` for a queued translation,
   *  and the settled source rung (authored | checked | approved) for a source
   *  unit. */
  status?: string;
  /** A source unit ranked below the project's source gate, so the loop holds
   *  its translations. */
  held?: boolean;
  /** Parent content-collection name (empty/absent for a bare entry). */
  collection?: string;
  /** The SOURCE file's project-relative path. */
  relative?: string;
  /** The project's source language, for the source preview's direction. */
  sourceLocale?: string;
  source: string;
  target?: string;
  /** Whether the unit currently trips a check, set by ReviewQueue's
   * enrichment. Absent when not computed, as it always is for a source row. */
  hasFindings?: boolean;
  /** AI pre-review score (0–100) when a fresh annotation exists for the
   * current translation — read from the state store, never a live call. */
  aiScore?: number;
  /** Model that produced aiScore. */
  aiModel?: string;
}

/** One language present in the review queue (matches Go
 *  convergence.ReviewLanguage). */
export interface ReviewLanguage {
  language: string;
  pending: number;
  /** The project's source language. */
  source?: boolean;
}

/** The review queue as a whole: every unit awaiting a person across the
 *  project's languages, and the languages they belong to (matches Go
 *  convergence.ReviewQueue). */
export interface ReviewQueue {
  pending: ReviewItem[] | null;
  languages: ReviewLanguage[] | null;
}

/** Provenance of a translation (matches Go model.Origin). */
export interface TargetOrigin {
  kind?: string; // human | tm | mt | ai | ocr | asr
  engine?: string;
  tool?: string;
  reference?: string;
  timestamp?: string;
  confidence?: number;
}

/** Full review picture for one queue unit (matches Go ReviewUnitDetail). */
/** What affordance the UI should render for a remedy. Mirrors backend.RunErrorActionKind. */
export type RunErrorActionKind = "command" | "open-settings" | "open-url";

/** One thing the user can do about a run failure. Mirrors backend.RunErrorAction. */
export interface RunErrorAction {
  kind: RunErrorActionKind;
  label: string;
  command?: string;
  url?: string;
  target?: string;
}

/** Stable classification of a run failure. Mirrors backend.RunErrorKind. */
export type RunErrorKind =
  | "ollama-unreachable"
  | "model-not-installed"
  | "ambiguous-credential"
  | "missing-credential"
  | "canceled"
  | "blocked-target-path"
  | "unknown";

/**
 * A run failure as structure rather than prose (backend.RunError): a headline,
 * the remediation with concrete actions, the affected file/locale, and the raw
 * wrapped chain for a details disclosure.
 */
export interface RunError {
  kind: RunErrorKind;
  headline: string;
  remediation?: string;
  actions?: RunErrorAction[];
  locale?: string;
  file?: string;
  provider?: string;
  model?: string;
  raw: string;
}

export interface ReviewUnitDetail {
  locale: string;
  file: string;
  key: string;
  collection?: string;
  source: string;
  target: string;
  /** The project's source language — drives `dir`/`lang` on the source pane. */
  source_locale?: string;
  /** Effective ladder state (draft|translated|reviewed|signed-off). */
  status: string;
  /** Last recorded decision when it still judges the current translation. */
  review_state?: string;
  note?: string;
  origin?: TargetOrigin;
  /** Best content-memory match percent (absent/0 = none found or no project content memory open). */
  tm_score?: number;
  findings: DesktopFinding[];
  /** Whether the target is a single plain-text run (safe to edit in place). */
  editable: boolean;
  /** Fresh AI pre-review annotation (state-store read; no provider call). */
  ai_review_score?: number;
  ai_review_model?: string;
  /** Everything that makes the decision judgeable, assembled by the host and
   *  served identically to every review client. */
  context?: ReviewContext;
}

/**
 * The review model (host.ReviewContext, core/review): one decision bound to
 * the context it is made in. Generated from the Go structs with
 * `make generate-contract-types`, so the desktop reads the shape the host
 * serialises and the platform's review surfaces read the same one.
 */
export type {
  ReviewContext,
  ReviewPoint,
  ReviewVoice,
  ReviewProfileValidity,
  ReviewNeighbourhood,
  ReviewNeighbour,
  ReviewHistory,
  ReviewPriorVersion,
  ReviewMemoryMatch,
  ReviewJudgement,
  ReviewProvenance,
  AIReviewFinding,
} from "@neokapi/contract-types";

/** One per-unit review AI action (matches Go backend constants). */
export type ReviewAIActionKind = "fix-findings" | "retranslate" | "explain";

/** Outcome of a per-unit review AI action: a proposed target (fix-findings /
 * retranslate — nothing written until accepted) or explanation text. */
export interface ReviewAIActionResult {
  proposed_target?: string;
  explanation?: string;
  /** The LLM calls this action made, so the reviewer can read what was sent
   *  before accepting what came back. */
  exchanges?: AIActivityEntry[];
}

/** One content part of a message: text, or a media reference. */
export interface AIContentPart {
  kind: string;
  text?: string;
  media?: { mime?: string; path?: string };
}

/** One message in an LLM exchange, as it went on the wire. A message is always
 *  a list of parts — there is no plain-string form. */
export interface AIMessage {
  role: string;
  parts?: AIContentPart[];
  /** Which prompt ingredient produced each piece of the text (framework rule,
   *  voice profile, terms, the source block), when a prompt builder made it. */
  sections?: { label?: string; text?: string }[];
}

/** One LLM call: what was sent, what constrained the output, what came back. */
export interface AIExchange {
  prompt?: string;
  prompt_version?: string;
  provider: string;
  model?: string;
  messages: AIMessage[];
  schema?: Record<string, unknown>;
  response?: string;
  usage?: {
    input_tokens?: number;
    output_tokens?: number;
    cache_read_tokens?: number;
    cache_write_tokens?: number;
  };
  error?: string;
}

/** Which piece of work an LLM call was made for. */
export interface AIActivityScope {
  surface: string;
  action?: string;
  locale?: string;
  file?: string;
  key?: string;
}

/** One recorded LLM call in the session's activity log. */
export interface AIActivityEntry {
  id: number;
  at: string;
  scope: AIActivityScope;
  provider: string;
  model?: string;
  prompt?: string;
  prompt_version?: string;
  error?: string;
  exchange: AIExchange;
}

/** The session's AI activity log, newest first. */
export interface AIActivityResult {
  entries: AIActivityEntry[];
  /** How many older entries the window evicted; non-zero means the list is
   *  partial. */
  dropped: number;
  /** The window size. */
  cap: number;
}

/** Narrowing for an AI pre-review run. */
export interface PreReviewScope {
  collection?: string;
}

/** What an AI pre-review may do: annotate-only (default) or auto-approve
 * units at/above minScore with no blocking check findings. */
export interface PreReviewPolicy {
  autoApprove: boolean;
  minScore: number;
}

/** Summary of an AI pre-review run. */
export interface PreReviewResult {
  model: string;
  reviewed: number;
  auto_approved: number;
  remaining: number;
  skipped?: number;
}

/** The full derived convergence picture for a project. */
export interface ConvergenceReport {
  project?: string;
  source?: SourceCoverage;
  locales: LocaleCoverage[];
  review: ReviewItem[];
}

// --- Convergence pre-flight plan + run result (the shared `kapi up` engine) ---

/** One (collection, locale) scope of the dry-run work plan (cli.UpPlanScope). */
export interface UpPlanScope {
  locale?: string;
  collection?: string;
  /** Translatable units with no committed target for the locale. */
  missingTarget: number;
  /** Units covered by an exact-hash content-memory hit. The wire name is the
   *  backend's JSON tag, which the rename boundary leaves as it was. */
  tmExact: number;
  /** Units the pass serves from the project store's stored drafts: a
   *  translation of the same source made under the configuration and context
   *  the run would use, so no provider call and no tokens. */
  drafts?: number;
  /** Units left for AI translation after content-memory leverage and stored
   *  drafts. */
  aiRemaining: number;
  /** Produced units whose decision blessed source wording that has since
   *  changed: the pass re-drafts them and they return to review. */
  stale?: number;
  /** Produced units the content memory does not answer — `recycle` cannot fill
   *  them, so the pass drafts over what is on disk. */
  unanswered?: number;
  /** Produced units the plan declines to judge because the project store has
   *  not read their committed translations yet. */
  unreadTargets?: number;
  /** Rough input-token estimate for the remaining AI work (chars/4). */
  tokenEstimate: number;
}

/** The dry-run plan `kapi up --plan` computes (cli.UpPlanOutput). */
export interface UpPlanOutput {
  flow?: string;
  scopes: UpPlanScope[] | null;
  totals: UpPlanScope;
  /** The AI provider a converge run would use (shared ai.provider default). */
  provider?: string;
  /** True when that provider bills a personal subscription (claude-code) —
   * the token estimate is scale, not a metered API cost. */
  subscription?: boolean;
  /** Discloses the estimation heuristic (Memory exact-hash only, chars/4 tokens). */
  note: string;
}

/** Desktop pre-flight picture: the work plan + the block-store drift the
 * run's auto-extract would heal (backend ConvergePlan). */
export interface ConvergePlan {
  plan: UpPlanOutput;
  changedFiles: number;
  removedFiles: number;
  storeMissing: boolean;
  versionStale: boolean;
}

// One typed progress event of a convergence run (core/convergence.Event) — the
// single protocol every venue speaks (CLI live renderer, `kapi up --json`, the
// server's SSE stream, and this desktop run view). The type and the one fold
// over it are shared with bowrain via @neokapi/status-views.
export type { ConvergenceEvent as ConvergeEvent } from "@neokapi/status-views";

/**
 * Where a project's `kapi up` runs (backend ProjectServer). A recipe with a
 * `bowrain:` block is Bowrain-connected — the canonical run executes on the
 * server; the desktop still runs the local engine, so the UI discloses the
 * venue rather than implying a remote run.
 */
export interface ProjectServer {
  connected: boolean;
  url?: string;
  /** Server host (no scheme) — the short label a venue badge renders. */
  host?: string;
  serverURL?: string;
  /** The stream the project reads and writes on. */
  stream?: string;
}

/** A gated (collection, locale) scope still short of its gate after a run. */
export interface ParkedScope {
  locale: string;
  collection?: string;
}

/** Per-locale outcome of a convergence run (cli.ConvergeLocaleResult). */
export interface ConvergeLocaleResult {
  locale: string;
  shippable: boolean;
  parked?: boolean;
  pct?: Record<string, number>;
  failingChecks?: number;
  materialized?: number;
}

/** Structured result of a convergence run (cli.ConvergeOutput). */
export interface ConvergeOutput {
  flow: string;
  passes: number;
  converged: boolean;
  locales: ConvergeLocaleResult[];
  parkedScopes?: ParkedScope[];
  materializedFiles?: number;
  /** Translatable source blocks held below the source gate: their translations
   *  were not produced because the source is unsettled. Source-scoped, so it is
   *  one count for every language rather than one per language. */
  blockedOnSource?: number;
  /** The resolved source gate the run applied (authored|checked|approved). */
  sourceGate?: string;
  /** Why the run did not converge, when it did not. `source_not_ready` means
   *  every pending locale had nothing producible. */
  stallReason?: string;
}

/** One skipped file from an extraction request. */
export interface ExtractSkip {
  path: string;
  reason: string;
}

/** Outcome of a project extraction request. */
export interface ExtractResult {
  files: number;
  blocks: number;
  skipped?: ExtractSkip[];
  log: string;
}

/** Outcome of adopting a user/ad-hoc flow into a project recipe. */
export interface AdoptFlowResult {
  name: string;
  /** true when the flow was renamed to avoid a clash with an existing one. */
  renamed: boolean;
}

/** Project-scoped resource handles ("" when none). */
export interface ProjectHandles {
  tabID: string;
  memoryHandle: string;
  termsHandle: string;
}

// Sidebar items for Ad-Hoc mode
//
// The `"termbases"` and `"memories"` ids keep their historical spellings on
// purpose. A view id is not display vocabulary — it is persisted: it round-trips
// through SessionState (see backend/settings.go) into the user's settings.json,
// and the next launch restores the view by id. Renaming one to match the "terms"
// / "content memory" vocabulary would silently fail to restore the tab for
// everyone who already has the old id saved. The sidebar *labels* are the thing
// users read, and those do say "Terms" and "Content Memory"; leave these alone.
export type AdhocView =
  | "home"
  | "flows"
  | "tools"
  | "termbases"
  | "memories"
  | "formats"
  | "settings";

// Sidebar items for Projects mode. Same persisted-id rule as AdhocView above.
export type ProjectView =
  | "home"
  | "content"
  | "flows"
  | "tools"
  | "checks"
  | "review"
  | "termbases"
  | "memories"
  | "settings";

// Union for convenience
export type View = AdhocView | ProjectView;

// --- Helper functions for collections ---

/** Check if a collection is a bare entry (has path, no content). */
export function isBareEntry(c: Collection): boolean {
  return !!c.path && (!c.content || c.content.length === 0);
}

/**
 * Get effective items for a collection: bare entries wrap as a single-item
 * array, and the collection's base is folded into every path, target and item
 * base, so callers see project-relative paths. Mirrors Go's
 * Collection.EffectiveItems.
 */
export function effectiveItems(c: Collection): ContentItem[] {
  const items: ContentItem[] = isBareEntry(c)
    ? [{ path: c.path!, format: c.format, target: c.target }]
    : (c.content ?? []);
  const base = c.base;
  if (!base) return items;
  return items.map((item) => ({
    ...item,
    path: joinBase(base, item.path),
    target: item.target ? joinBase(base, item.target) : item.target,
    base: item.base ? joinBase(base, item.base) : item.base,
  }));
}

/** Prefix a collection-relative path with the collection's base. Mirrors Go's
 * project.JoinBase: an empty or absolute path is left alone. */
function joinBase(base: string, p: string): string {
  if (!p || p.startsWith("/")) return p;
  return base.replace(/\/+$/, "") + "/" + p;
}
