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
  description: string;
  category: string;
  has_schema: boolean;
}

export interface FormatInfo {
  name: string;
  description: string;
  extensions: string[];
}

export interface PluginInfo {
  name: string;
  version: string;
  description: string;
  type: string;
  installed: boolean;
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

export type View =
  | "welcome"
  | "project"
  | "flows"
  | "tools"
  | "plugins"
  | "settings";
