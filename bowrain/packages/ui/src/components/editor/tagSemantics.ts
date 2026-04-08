// Re-export from shared package — implementation lives in @neokapi/ui-primitives.
export {
  tagNameFromData,
  semanticCategory,
  semanticLabel,
  semanticTooltip,
  tagColors,
  buildPairs,
  validateTags,
  codedTextToHtml,
} from "@neokapi/ui-primitives";
export type { TagColorScheme, TagValidationResult } from "@neokapi/ui-primitives";
