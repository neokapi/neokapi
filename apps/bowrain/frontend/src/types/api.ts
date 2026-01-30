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
}

/** Flow descriptor */
export interface FlowInfo {
  name: string;
  description: string;
}

/** Plugin descriptor */
export interface PluginInfo {
  name: string;
  type: string;
  source: string;
  formats: string[];
}

/** Parameters for format conversion */
export interface ConvertRequest {
  input_path: string;
  output_path: string;
  input_format?: string;
  output_format?: string;
  source_lang?: string;
  target_lang?: string;
  encoding?: string;
}

/** Parameters for AI translation */
export interface TranslateRequest {
  input_path: string;
  output_path?: string;
  format?: string;
  source_lang?: string;
  target_lang: string;
  provider?: string;
  api_key?: string;
  model?: string;
  encoding?: string;
}

/** Parameters for flow execution */
export interface FlowRequest {
  flow_name: string;
  input_path: string;
  output_path?: string;
  format?: string;
  source_lang?: string;
  target_lang: string;
  provider?: string;
  api_key?: string;
  model?: string;
  encoding?: string;
}

/** Result of a conversion operation */
export interface ConvertResult {
  output_path: string;
  part_count: number;
}

/** Result of a translation operation */
export interface TranslateResult {
  output_path: string;
  block_count: number;
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
  files: ProjectFile[];
  created_at: string;
  modified_at: string;
}

/** File within a project */
export interface ProjectFile {
  name: string;
  format: string;
  size: number;
  block_count: number;
  word_count: number;
}

/** Translation block info */
export interface BlockInfo {
  id: string;
  source: string;
  targets: Record<string, string>;
  translatable: boolean;
  has_spans: boolean;
  properties: Record<string, string>;
}

/** Update block request */
export interface UpdateBlockRequest {
  project_id: string;
  file_name: string;
  block_id: string;
  target_locale: string;
  text: string;
}

/** AI translate file request */
export interface AITranslateFileRequest {
  project_id: string;
  file_name: string;
  target_locale: string;
  provider: string;
  api_key: string;
  model: string;
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
