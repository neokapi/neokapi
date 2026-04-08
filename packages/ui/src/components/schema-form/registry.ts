const WIDGET_ALIASES: Record<string, string> = {
  multilineText: "textarea",
  codeFinderRules: "code-finder",
  simplifierRulesEditor: "simplifier-rules",
  elementRulesEditor: "element-rules",
  attributeRulesEditor: "attribute-rules",
  regexBuilder: "regex",
  tagList: "tags",
  numberList: "number-list",
  checkList: "checklist",
};

export const WIDGET_NAMES = [
  "text",
  "textarea",
  "password",
  "code-editor",
  "regex",
  "tags",
  "number-list",
  "segmented",
  "file-picker",
  "folder-picker",
  "checklist",
  "select",
  "credential-picker",
  "code-finder",
  "element-rules",
  "attribute-rules",
  "simplifier-rules",
  "path",
  "folder",
] as const;

export type WidgetName = (typeof WIDGET_NAMES)[number];

export function resolveWidgetName(name: string | undefined): string | undefined {
  if (!name) return undefined;
  return WIDGET_ALIASES[name] ?? name;
}
