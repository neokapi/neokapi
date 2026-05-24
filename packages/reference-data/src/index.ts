// @neokapi/reference-data — generated reference dataset for built-in and
// okapi-bridge formats and tools, plus the kapi CLI command reference.
// Regenerate with `make generate-reference-docs` (scripts/gen-refs).
// Do not edit the JSON under data/ by hand.

import formatsJson from "../data/formats.json";
import toolsJson from "../data/tools.json";
import gapsJson from "../data/reference-gaps.json";
import commandsJson from "../data/commands.json";
import type { ReferenceDataset, ReferenceEntry, ReferenceGapReport, CommandDataset } from "./types";

export * from "./types";

export const formats = formatsJson as unknown as ReferenceDataset;
export const tools = toolsJson as unknown as ReferenceDataset;
export const gaps = gapsJson as unknown as ReferenceGapReport;
export const commands = commandsJson as unknown as CommandDataset;

/** All formats and tools in one array. */
export function allEntries(): ReferenceEntry[] {
  return [...formats.entries, ...tools.entries];
}
