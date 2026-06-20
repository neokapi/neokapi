import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { SchemaForm } from "@neokapi/ui-primitives";
import type { ComponentSchema } from "../types";

function SchemaFormWrapper({
  schema,
  initialValues = {},
  compact = false,
  width = 360,
  presetValues,
}: {
  schema: ComponentSchema;
  initialValues?: Record<string, unknown>;
  compact?: boolean;
  width?: number;
  presetValues?: Record<string, unknown>;
}) {
  const [values, setValues] = useState<Record<string, unknown>>(initialValues);
  return (
    <div style={{ maxWidth: width }}>
      <SchemaForm
        schema={schema}
        values={values}
        onChange={setValues}
        compact={compact}
        presetValues={presetValues}
      />
      <pre
        style={{
          marginTop: 16,
          padding: 12,
          borderRadius: 6,
          background: "oklch(0.17 0.012 260)",
          fontSize: 11,
          color: "oklch(0.7 0.01 260)",
          overflow: "auto",
          maxHeight: 200,
        }}
      >
        {JSON.stringify(values, null, 2)}
      </pre>
    </div>
  );
}

const pseudoTranslateSchema: ComponentSchema = {
  title: "Pseudo Translate",
  type: "object",
  toolMeta: { id: "pseudo-translate", category: "transform" },
  "ui:groups": [
    { id: "output", label: "Output Format", fields: ["prefix", "suffix", "expansionPercent"] },
    { id: "behavior", label: "Behavior", fields: ["applyAccents", "padWithX"] },
  ],
  properties: {
    prefix: {
      type: "string",
      title: "Prefix",
      default: "[",
      description: "Prefix added to translations",
    },
    suffix: {
      type: "string",
      title: "Suffix",
      default: "]",
      description: "Suffix added to translations",
    },
    expansionPercent: {
      type: "integer",
      title: "Expansion Percent",
      default: 30,
      minimum: 0,
      maximum: 200,
      description: "Expand text length by percentage",
    },
    applyAccents: {
      type: "boolean",
      title: "Apply Accents",
      default: true,
      description: "Apply diacritical marks to simulate translated text",
    },
    padWithX: {
      type: "boolean",
      title: "Pad with X",
      default: false,
      description: "Pad expansion with 'x' characters instead of spaces",
    },
  },
};

const qaCheckSchema: ComponentSchema = {
  title: "QA Check",
  type: "object",
  toolMeta: { id: "qa", category: "validate" },
  "ui:groups": [
    {
      id: "whitespace",
      label: "Whitespace Checks",
      fields: ["checkLeadingWhitespace", "checkTrailingWhitespace", "checkDoubleSpaces"],
    },
    {
      id: "content",
      label: "Content Checks",
      fields: ["checkMissingTranslation", "checkInlineCodes", "checkPatterns"],
    },
  ],
  properties: {
    checkLeadingWhitespace: {
      type: "boolean",
      title: "Check Leading Whitespace",
      default: true,
      description: "Check for leading whitespace mismatches",
    },
    checkTrailingWhitespace: {
      type: "boolean",
      title: "Check Trailing Whitespace",
      default: true,
      description: "Check trailing whitespace",
    },
    checkDoubleSpaces: {
      type: "boolean",
      title: "Check Double Spaces",
      default: true,
      description: "Flag double spaces in target",
    },
    checkMissingTranslation: {
      type: "boolean",
      title: "Check Missing Translation",
      default: true,
      description: "Flag empty translations",
    },
    checkInlineCodes: {
      type: "boolean",
      title: "Check Inline Codes",
      default: true,
      description: "Verify inline codes are preserved",
    },
    checkPatterns: {
      type: "boolean",
      title: "Check Patterns",
      default: false,
      description: "Check for pattern mismatches",
    },
    severityLevel: {
      type: "string",
      title: "Severity Level",
      default: "warning",
      enum: ["error", "warning", "info"],
      description: "Default severity level",
    },
    maxIssues: {
      type: "integer",
      title: "Max Issues",
      default: 100,
      minimum: 1,
      maximum: 10000,
      description: "Maximum issues to report",
    },
  },
};

const searchReplaceSchema: ComponentSchema = {
  title: "Search and Replace",
  type: "object",
  toolMeta: { id: "search-replace", category: "transform" },
  properties: {
    search: { type: "string", title: "Search", description: "Search pattern or literal text" },
    replace: { type: "string", title: "Replace", description: "Replacement text" },
    regEx: {
      type: "boolean",
      title: "Regular Expression",
      default: false,
      description: "Treat search as a regular expression",
    },
    target: {
      type: "boolean",
      title: "Apply to Target",
      default: true,
      description: "Apply to target text",
    },
    source: {
      type: "boolean",
      title: "Apply to Source",
      default: false,
      description: "Apply to source text",
    },
    dotAll: {
      type: "boolean",
      title: "Dot All",
      default: false,
      description: "Dot matches newlines in regex",
    },
    caseInsensitive: {
      type: "boolean",
      title: "Case Insensitive",
      default: false,
      description: "Case-insensitive matching",
    },
  },
};

// ── New: schemas exercising object/array/widget types ──

const nestedObjectSchema: ComponentSchema = {
  title: "HTML Filter",
  type: "object",
  formatMeta: { id: "html-filter" },
  "ui:groups": [
    { id: "parser", label: "Parser Settings", fields: ["parser"] },
    { id: "extraction", label: "Extraction", fields: ["elements", "attributes"] },
    { id: "inline", label: "Inline Codes", fields: ["inlineCodes"] },
  ],
  properties: {
    parser: {
      type: "object",
      title: "Parser",
      description: "Parser behavior settings",
      properties: {
        assumeWellformed: {
          type: "boolean",
          title: "Assume Wellformed",
          default: false,
          description:
            "Assume input HTML is well-formed XML. Faster but may fail on non-conforming HTML.",
        },
        preserveWhitespace: {
          type: "boolean",
          title: "Preserve Whitespace",
          default: false,
          description: "Preserve original whitespace in extracted text",
        },
      },
    },
    elements: {
      type: "object",
      title: "Elements",
      description: "Element extraction rules",
      "ui:widget": "elementRulesEditor",
      additionalProperties: {
        type: "object",
        properties: {
          ruleTypes: {
            type: "array",
            title: "Rule Types",
            items: {
              type: "string",
              enum: ["INLINE", "TEXTUNIT", "EXCLUDE", "INCLUDE", "ATTRIBUTES_ONLY"],
            },
          },
        },
      } as unknown as boolean,
    },
    attributes: {
      type: "object",
      title: "Attributes",
      description: "Global attribute extraction rules",
      "ui:widget": "attributeRulesEditor",
      additionalProperties: {
        type: "object",
        properties: {
          ruleTypes: {
            type: "array",
            title: "Rule Types",
            items: {
              type: "string",
              enum: ["ATTRIBUTE_TRANS", "ATTRIBUTE_WRITABLE", "ATTRIBUTE_READONLY", "ATTRIBUTE_ID"],
            },
          },
          allElementsExcept: {
            type: "array",
            title: "All Elements Except",
            items: { type: "string" },
          },
        },
      } as unknown as boolean,
    },
    inlineCodes: {
      type: "object",
      title: "Inline Codes",
      description: "Inline code detection and handling",
      "ui:widget": "codeFinderRules",
      "ui:presets": {
        "HTML Tags": { rules: [{ pattern: "</?\\w[^>]*>" }] },
        "Printf Placeholders": { rules: [{ pattern: "%[\\w.]*[dsfx]" }] },
      },
    },
    editorTitle: { type: "string", title: "Editor Title", description: "Display title in editor" },
    simplifierRules: {
      type: "string",
      title: "Simplifier Rules",
      "ui:widget": "simplifierRulesEditor",
      description: "Rules for simplifying inline code representation",
    },
  },
};

const arraySchema: ComponentSchema = {
  title: "Fixed-Width Columns",
  type: "object",
  formatMeta: { id: "fixed-width" },
  properties: {
    columns: {
      type: "array",
      title: "Columns",
      description: "Column definitions for fixed-width parsing",
      items: {
        type: "object",
        properties: {
          name: { type: "string", title: "Name", description: "Column name" },
          start: {
            type: "integer",
            title: "Start",
            minimum: 0,
            description: "Start position (0-based)",
          },
          width: { type: "integer", title: "Width", minimum: 1, description: "Column width" },
          translatable: {
            type: "boolean",
            title: "Translatable",
            default: false,
            description: "Whether to extract for translation",
          },
        },
      },
    },
    encoding: {
      type: "string",
      title: "Encoding",
      default: "UTF-8",
      enum: ["UTF-8", "UTF-16", "ISO-8859-1", "Windows-1252"],
      description: "File character encoding",
    },
    skipLines: {
      type: "integer",
      title: "Skip Lines",
      default: 0,
      minimum: 0,
      description: "Header lines to skip",
    },
  },
};

const simpleArraySchema: ComponentSchema = {
  title: "Regex Filter",
  type: "object",
  formatMeta: { id: "regex-filter" },
  properties: {
    patterns: {
      type: "array",
      title: "Patterns",
      description: "Extraction patterns (regex)",
      items: { type: "string" },
    },
    caseSensitive: { type: "boolean", title: "Case Sensitive", default: true },
  },
};

const jsonFallbackSchema: ComponentSchema = {
  title: "Custom Config",
  type: "object",
  toolMeta: { id: "custom", category: "pipeline" },
  properties: {
    name: { type: "string", title: "Name", description: "Configuration name" },
    settings: {
      type: "object",
      title: "Settings",
      description: "Arbitrary settings (JSON)",
    },
    tags: {
      type: "array",
      title: "Tags",
      description: "Tags for this configuration",
    },
  },
};

const showIfSchema: ComponentSchema = {
  title: "Conditional Fields",
  type: "object",
  toolMeta: { id: "conditional", category: "transform" },
  properties: {
    mode: {
      type: "string",
      title: "Mode",
      default: "simple",
      enum: ["simple", "advanced"],
      description: "Processing mode",
    },
    threshold: {
      type: "number",
      title: "Threshold",
      default: 0.8,
      description: "Match threshold (only in advanced mode)",
      "ui:visible": { field: "mode", eq: "advanced" },
    },
    maxResults: {
      type: "integer",
      title: "Max Results",
      default: 100,
      description: "Maximum results (only in advanced mode)",
      "ui:visible": { field: "mode", eq: "advanced" },
    },
    caseSensitive: {
      type: "boolean",
      title: "Case Sensitive",
      default: false,
      description: "Case-sensitive matching",
    },
  },
};

// ── Deeply nested schema (3+ depth) ──

const deeplyNestedSchema: ComponentSchema = {
  title: "HTML Format",
  type: "object",
  "ui:groups": [
    { id: "parser", label: "Parser Settings", fields: ["preserveWhitespace"] },
    { id: "extraction", label: "Extraction Rules", fields: ["elements", "attributes"] },
    { id: "codes", label: "Inline Codes", fields: ["codeFinderRules", "useCodeFinder"] },
  ],
  properties: {
    preserveWhitespace: {
      type: "boolean",
      title: "Preserve Whitespace",
      default: false,
      description: "Preserve significant whitespace in text nodes",
    },
    elements: {
      type: "object",
      title: "Elements",
      description: "Map of element names to extraction rules",
      additionalProperties: {
        type: "object",
        properties: {
          ruleType: {
            type: "string",
            title: "Rule Type",
            enum: ["INLINE", "GROUP", "EXCLUDE", "TEXTUNIT", "PRESERVE_WHITESPACE"],
            default: "INLINE",
          },
          translatable: { type: "boolean", title: "Translatable", default: true },
        },
      } as unknown as boolean,
    },
    attributes: {
      type: "object",
      title: "Attributes",
      description: "Map of attribute names to extraction rules",
      additionalProperties: { type: "string" } as unknown as boolean,
    },
    codeFinderRules: {
      type: "object",
      title: "Code Finder Rules",
      description: "Rules for identifying inline codes",
      properties: {
        useAllRulesWhenTesting: {
          type: "boolean",
          title: "Use All Rules When Testing",
          default: true,
        },
        includes: {
          type: "array",
          title: "Includes",
          items: { type: "string" },
          description: "Regex patterns to include as inline codes",
        },
        excludes: {
          type: "array",
          title: "Excludes",
          items: { type: "string" },
          description: "Regex patterns to exclude from inline codes",
        },
      },
    },
    useCodeFinder: {
      type: "boolean",
      title: "Use Code Finder",
      default: true,
      description: "Enable the inline code finder",
    },
  },
};

// ── Map editor schema ──

const mapEditorSchema: ComponentSchema = {
  title: "Environment Variables",
  type: "object",
  toolMeta: { id: "env-vars", category: "config" },
  "ui:groups": [
    { id: "maps", label: "Variable Maps", fields: ["variables", "secrets", "overrides"] },
    { id: "options", label: "Options", fields: ["expandVars", "caseSensitiveKeys"] },
  ],
  properties: {
    variables: {
      type: "object",
      title: "Variables",
      description: "Environment variables (key-value pairs)",
      additionalProperties: { type: "string" } as unknown as boolean,
    },
    secrets: {
      type: "object",
      title: "Secrets",
      description: "Secret variables (masked in output)",
      additionalProperties: { type: "string" } as unknown as boolean,
    },
    overrides: {
      type: "object",
      title: "Overrides",
      description: "Per-locale variable overrides",
      additionalProperties: {
        type: "object",
        properties: {
          value: { type: "string", title: "Value", description: "Override value" },
          locales: { type: "string", title: "Locales", description: "Comma-separated locale list" },
        },
      } as unknown as boolean,
    },
    expandVars: {
      type: "boolean",
      title: "Expand Vars",
      default: true,
      description: "Expand ${VAR} references in values",
    },
    caseSensitiveKeys: {
      type: "boolean",
      title: "Case Sensitive Keys",
      default: true,
      description: "Treat variable names as case-sensitive",
    },
  },
};

// ── Formats page config schema (wide container) ──

const formatsPageSchema: ComponentSchema = {
  title: "JSON Format",
  type: "object",
  formatMeta: { id: "json-format" },
  "ui:groups": [
    { id: "parsing", label: "Parsing", fields: ["keyStyle", "arrayHandling", "preserveOrder"] },
    { id: "extraction", label: "Extraction", fields: ["extractPaths", "excludePaths"] },
    { id: "output", label: "Output", fields: ["indentation", "trailingNewline", "sortKeys"] },
  ],
  properties: {
    keyStyle: {
      type: "string",
      title: "Key Style",
      default: "nested",
      enum: ["nested", "flat", "auto"],
      description: "How to interpret JSON key paths",
    },
    arrayHandling: {
      type: "string",
      title: "Array Handling",
      default: "index",
      enum: ["index", "content", "skip"],
      description: "How to handle array elements",
    },
    preserveOrder: {
      type: "boolean",
      title: "Preserve Order",
      default: true,
      description: "Preserve original key order in output",
    },
    extractPaths: {
      type: "array",
      title: "Extract Paths",
      items: { type: "string" },
      description: "JSON paths to extract (empty = extract all)",
    },
    excludePaths: {
      type: "array",
      title: "Exclude Paths",
      items: { type: "string" },
      description: "JSON paths to exclude from extraction",
    },
    indentation: {
      type: "integer",
      title: "Indentation",
      default: 2,
      minimum: 0,
      maximum: 8,
      description: "Number of spaces for indentation",
    },
    trailingNewline: {
      type: "boolean",
      title: "Trailing Newline",
      default: true,
      description: "Add trailing newline to output",
    },
    sortKeys: {
      type: "boolean",
      title: "Sort Keys",
      default: false,
      description: "Alphabetically sort keys in output",
    },
  },
};

const meta: Meta<typeof SchemaFormWrapper> = {
  title: "Flow Editor/SchemaForm",
  component: SchemaFormWrapper,
  tags: ["autodocs"],
  parameters: { layout: "padded" },
};

export default meta;
type Story = StoryObj<typeof SchemaFormWrapper>;

export const PseudoTranslate: Story = {
  args: { schema: pseudoTranslateSchema },
};

export const PseudoTranslateWithValues: Story = {
  args: {
    schema: pseudoTranslateSchema,
    initialValues: { prefix: ">>", suffix: "<<", expansionPercent: 45, applyAccents: false },
  },
};

export const QACheck: Story = {
  args: { schema: qaCheckSchema },
};

export const SearchReplace: Story = {
  args: { schema: searchReplaceSchema },
};

export const Compact: Story = {
  args: {
    schema: qaCheckSchema,
    compact: true,
  },
};

// ── New stories for complex types ──

export const NestedObject: Story = {
  name: "Nested Object (HTML Filter)",
  args: { schema: nestedObjectSchema },
};

export const NestedObjectWithValues: Story = {
  name: "Nested Object (with values)",
  args: {
    schema: nestedObjectSchema,
    initialValues: {
      parser: { assumeWellformed: true, preserveWhitespace: false },
      elements: { meta: { ruleTypes: ["ATTRIBUTES_ONLY"] }, script: { ruleTypes: ["EXCLUDE"] } },
      inlineCodes: { rules: [{ pattern: "</?\\w[^>]*>" }], sample: "<b>bold</b> text" },
    },
  },
};

export const ArrayOfObjects: Story = {
  name: "Array of Objects (Fixed-Width)",
  args: {
    schema: arraySchema,
    initialValues: {
      columns: [
        { name: "id", start: 0, width: 10, translatable: false },
        { name: "text", start: 10, width: 50, translatable: true },
      ],
    },
  },
};

export const SimpleArray: Story = {
  name: "Array of Strings (Regex)",
  args: {
    schema: simpleArraySchema,
    initialValues: {
      patterns: ['^\\s*msgid\\s*"(.*)"', '^\\s*msgstr\\s*"(.*)"'],
    },
  },
};

export const JsonFallback: Story = {
  name: "JSON Fallback (bare object/array)",
  args: {
    schema: jsonFallbackSchema,
    initialValues: {
      name: "custom",
      settings: { timeout: 30, debug: true },
      tags: ["production", "v2"],
    },
  },
};

export const ConditionalFields: Story = {
  name: "ui:visible Conditional Visibility",
  args: {
    schema: showIfSchema,
    initialValues: { mode: "simple" },
  },
};

export const ConditionalFieldsAdvanced: Story = {
  name: "ui:visible (advanced mode)",
  args: {
    schema: showIfSchema,
    initialValues: { mode: "advanced", threshold: 0.75, maxResults: 50 },
  },
};

// ── New stories: deeply nested, map editor, formats page ──

export const DeeplyNestedConfig: Story = {
  name: "Deeply Nested Config (HTML Format)",
  args: {
    schema: deeplyNestedSchema,
    initialValues: {
      preserveWhitespace: false,
      elements: {
        div: { ruleType: "TEXTUNIT", translatable: true },
        span: { ruleType: "INLINE", translatable: true },
        script: { ruleType: "EXCLUDE", translatable: false },
      },
      attributes: {
        title: "translatable",
        alt: "translatable",
        placeholder: "translatable",
      },
      codeFinderRules: {
        useAllRulesWhenTesting: true,
        includes: ["</?\\w[^>]*>", "\\{\\{.*?\\}\\}"],
        excludes: ["<!--.*?-->"],
      },
      useCodeFinder: true,
    },
  },
};

export const MapEditorStory: Story = {
  name: "Map Editor (key-value maps)",
  args: {
    schema: mapEditorSchema,
    initialValues: {
      variables: {
        API_URL: "https://api.example.com",
        APP_NAME: "My App",
        VERSION: "2.1.0",
      },
      secrets: {
        API_KEY: "sk-***",
      },
      overrides: {},
      expandVars: true,
      caseSensitiveKeys: true,
    },
  },
};

export const FormatsPageConfig: Story = {
  name: "Formats Page Config (wide)",
  args: {
    schema: formatsPageSchema,
    width: 500,
    initialValues: {
      keyStyle: "nested",
      arrayHandling: "index",
      preserveOrder: true,
      extractPaths: ["$.messages", "$.labels"],
      excludePaths: ["$.internal"],
      indentation: 2,
      trailingNewline: true,
      sortKeys: false,
    },
  },
};

// ── Preset indicator story ──

export const WithPresetIndicator: Story = {
  name: "With Preset Indicator",
  args: {
    schema: pseudoTranslateSchema,
    initialValues: {
      prefix: ">>",
      suffix: "]",
      expansionPercent: 45,
      applyAccents: true,
      padWithX: false,
    },
    presetValues: {
      prefix: "[",
      suffix: "]",
      expansionPercent: 30,
      applyAccents: true,
      padWithX: false,
    },
  },
};
