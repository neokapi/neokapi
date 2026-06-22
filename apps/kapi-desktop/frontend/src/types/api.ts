// Types matching the Go backend structs exposed via Wails bindings.

// IOPort (and the schema-language cluster, re-exported below) is defined once in
// the shared @neokapi/contract-types package (issue #817).
import type { IOPort } from "@neokapi/contract-types";

export interface KapiProject {
  version: string;
  name: string;
  plugins?: Record<string, PluginSpec>;
  defaults?: ProjectDefaults;
  content?: ContentCollection[];
  preset?: string;
  flows?: Record<string, FlowSpec>;
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
}

export interface FormatDefaults {
  preset?: string;
  config?: Record<string, unknown>;
  priority?: number;
}

/**
 * A content collection is either a bare entry (has path, no items) or a
 * named collection (has name and items).
 */
export interface ContentCollection {
  // Collection fields (long form).
  name?: string;
  source_language?: string;
  target_languages?: string[];
  items?: ContentItem[];

  // Bare entry fields (short form — promoted from ContentItem).
  path?: string;
  format?: FormatSpec;
  target?: string;
  /** Directory the source path is made relative to for target resolution. */
  base?: string;

  // Optional archived-state marker; gates the Translation-state section in
  // ContentPage (absent on most collections).
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
  description?: string;
  steps: FlowStep[];
}

export interface FlowStep {
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

export interface FlowInfo {
  name: string;
  description: string;
  step_count: number;
  valid: boolean;
  issues?: FlowIssue[];
}

export type LocaleCardinality = "monolingual" | "bilingual" | "multilingual";

export interface ToolInfo {
  name: string;
  display_name?: string;
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
  /** Structured fix text (e.g. a brand profile's preferred term). */
  replacement?: string;
  /** Whether the panel may show a one-click "Apply fix" button. */
  fixable: boolean;
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
  /** Optional one-line rationale (recommended local models). */
  note?: string;
  /** The currently configured default. */
  is_default: boolean;
}

export interface TabInfo {
  id: string;
  name: string;
  path: string;
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

export interface FilterDoc {
  filterName: string;
  overview: string;
  filterId?: string;
  wikiUrl?: string;
  parameters?: Record<string, ParameterDoc>;
  limitations?: string[];
  processingNotes?: string[];
  examples?: DocExample[];
}

export interface StepDoc {
  filterName: string; // actually the step display name
  overview: string;
  stepId?: string;
  wikiUrl?: string;
  parameters?: Record<string, ParameterDoc>;
  limitations?: string[];
  processingNotes?: string[];
  examples?: DocExample[];
}

export interface ParameterDoc {
  description?: string;
  /** Alias for description used in okapi-bridge doc files. */
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
  /** Maps locale → translated block count. */
  coverage: Record<string, number>;
  targetLanguages: string[];
}

/** Project-wide extraction + coverage status. */
export interface ProjectStatus {
  projectPath: string;
  projectName: string;
  /** false ⇒ never extracted; show the shell + a "run extract" prompt. */
  hasData: boolean;
  collections: CollectionStatus[];
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
  tmHandle: string;
  termbaseHandle: string;
}

// Sidebar items for Ad-Hoc mode
export type AdhocView =
  | "home"
  | "flows"
  | "tools"
  | "termbases"
  | "memories"
  | "formats"
  | "settings";

// Sidebar items for Projects mode
export type ProjectView =
  | "home"
  | "content"
  | "flows"
  | "tools"
  | "checks"
  | "termbases"
  | "memories"
  | "settings";

// Union for convenience
export type View = AdhocView | ProjectView;

// --- Helper functions for content collections ---

/** Check if a content collection is a bare entry (has path, no items). */
export function isBareEntry(c: ContentCollection): boolean {
  return !!c.path && (!c.items || c.items.length === 0);
}

/** Get effective items for a collection (wraps bare entries as single-item array). */
export function effectiveItems(c: ContentCollection): ContentItem[] {
  if (isBareEntry(c)) {
    return [{ path: c.path!, format: c.format, target: c.target }];
  }
  return c.items ?? [];
}
