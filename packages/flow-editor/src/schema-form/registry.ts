// registry.ts
// Maps canonical widget names to normalized names (handles legacy aliases)

const WIDGET_ALIASES: Record<string, string> = {
  "multilineText": "textarea",
  "codeFinderRules": "code-finder",
  "simplifierRulesEditor": "simplifier-rules",
  "elementRulesEditor": "element-rules",
  "attributeRulesEditor": "attribute-rules",
  "regexBuilder": "regex",
  "tagList": "tags",
  "numberList": "number-list",
  "checkList": "checklist",
};

/** Canonical widget names. */
export const WIDGET_NAMES = [
  "text", "textarea", "password", "code-editor", "regex", "tags",
  "number-list", "segmented", "file-picker", "folder-picker",
  "checklist", "select", "code-finder", "element-rules",
  "attribute-rules", "simplifier-rules", "path", "folder",
] as const;

export type WidgetName = typeof WIDGET_NAMES[number];

/** Resolve a widget name, handling legacy aliases. */
export function resolveWidgetName(name: string | undefined): string | undefined {
  if (!name) return undefined;
  return WIDGET_ALIASES[name] ?? name;
}
