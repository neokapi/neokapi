// @neokapi/reference-data — generated reference dataset for built-in and
// okapi-bridge formats and tools, plus the kapi CLI command reference.
// Regenerate with `make generate-reference-docs` (scripts/gen-refs).
// Do not edit the JSON under data/ by hand.

import formatsJson from "../data/formats.json";
import toolsJson from "../data/tools.json";
import gapsJson from "../data/reference-gaps.json";
import commandsJson from "../data/commands.json";
import promptsJson from "../data/prompts.json";
import modelsJson from "../data/models.json";
import mcpToolsJson from "../data/mcp-tools.json";
import type {
  ReferenceDataset,
  ReferenceEntry,
  ReferenceGapReport,
  CommandDataset,
  PromptDataset,
  ModelDataset,
  MCPDataset,
} from "./types";

export * from "./types";

export const formats = formatsJson as unknown as ReferenceDataset;
export const tools = toolsJson as unknown as ReferenceDataset;
export const gaps = gapsJson as unknown as ReferenceGapReport;
export const commands = commandsJson as unknown as CommandDataset;

/** Every prompt kapi sends to a language model, rendered from the builders the binary uses. */
export const prompts = promptsJson as unknown as PromptDataset;

/** The curated catalog of models kapi supports, with their neokapi lifecycle. */
export const models = modelsJson as unknown as ModelDataset;

/** The tools a live `kapi mcp` server answers to, per exposed surface. */
export const mcpTools = mcpToolsJson as unknown as MCPDataset;

/** All formats and tools in one array. */
export function allEntries(): ReferenceEntry[] {
  return [...formats.entries, ...tools.entries];
}
