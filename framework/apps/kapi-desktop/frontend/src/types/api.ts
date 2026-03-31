// Types matching the Go backend structs exposed via Wails bindings.

export interface KapiProject {
  version: string;
  name: string;
  source_language?: string;
  target_languages?: string[];
  content?: ContentEntry[];
  preset?: string;
  plugins?: string[];
  flows?: Record<string, FlowSpec>;
  defaults?: ProjectDefaults;
}

export interface ContentEntry {
  path: string;
  format?: string;
  target?: string;
  collection?: string;
  format_preset?: string;
  format_config?: Record<string, unknown>;
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

export interface ProjectDefaults {
  concurrency?: number;
  parallel_blocks?: number;
  encoding?: string;
}

export interface FlowInfo {
  name: string;
  description: string;
  step_count: number;
}

export interface ToolInfo {
  name: string;
  display_name?: string;
  description: string;
  category: string;
  has_schema: boolean;
  inputs?: string[];
  outputs?: string[];
  tags?: string[];
  requires?: string[];
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

export interface ProviderConfig {
  id: string;
  name: string;
  provider_type: string;
  model?: string;
  base_url?: string;
}

export interface TabInfo {
  id: string;
  name: string;
  path: string;
}

// Schema types — re-exported from the flow-editor package (single source of truth)
export type {
  ComponentSchema,
  ComponentMeta,
  ParameterGroup,
  PropertySchema,
} from "@neokapi/flow-editor";

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
  description: string;
  notes?: string[];
  introducedIn?: string;
  dependsOn?: ParameterDependency[];
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
  | "termbases"
  | "memories"
  | "settings";

// Union for convenience
export type View = AdhocView | ProjectView;
