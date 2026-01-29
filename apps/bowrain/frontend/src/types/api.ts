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
