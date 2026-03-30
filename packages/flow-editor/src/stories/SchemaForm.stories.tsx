import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { SchemaForm } from "../SchemaForm";
import type { ComponentSchema } from "../types";

function SchemaFormWrapper({
  schema,
  initialValues = {},
  compact = false,
}: {
  schema: ComponentSchema;
  initialValues?: Record<string, unknown>;
  compact?: boolean;
}) {
  const [values, setValues] = useState<Record<string, unknown>>(initialValues);
  return (
    <div style={{ maxWidth: 360 }}>
      <SchemaForm schema={schema} values={values} onChange={setValues} compact={compact} />
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
  "x-component": { id: "pseudo-translate", type: "tool", category: "transform" },
  "x-groups": [
    { id: "output", label: "Output Format", fields: ["prefix", "suffix", "expansionPercent"] },
    { id: "behavior", label: "Behavior", fields: ["applyAccents", "padWithX"] },
  ],
  properties: {
    prefix: { type: "string", default: "[", description: "Prefix added to translations" },
    suffix: { type: "string", default: "]", description: "Suffix added to translations" },
    expansionPercent: {
      type: "integer",
      default: 30,
      minimum: 0,
      maximum: 200,
      description: "Expand text length by percentage",
    },
    applyAccents: {
      type: "boolean",
      default: true,
      description: "Apply diacritical marks to simulate translated text",
    },
    padWithX: {
      type: "boolean",
      default: false,
      description: "Pad expansion with 'x' characters instead of spaces",
    },
  },
};

const qaCheckSchema: ComponentSchema = {
  title: "QA Check",
  type: "object",
  "x-component": { id: "qa-check", type: "tool", category: "validate" },
  "x-groups": [
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
      default: true,
      description: "Check for leading whitespace mismatches",
    },
    checkTrailingWhitespace: {
      type: "boolean",
      default: true,
      description: "Check trailing whitespace",
    },
    checkDoubleSpaces: {
      type: "boolean",
      default: true,
      description: "Flag double spaces in target",
    },
    checkMissingTranslation: {
      type: "boolean",
      default: true,
      description: "Flag empty translations",
    },
    checkInlineCodes: {
      type: "boolean",
      default: true,
      description: "Verify inline codes are preserved",
    },
    checkPatterns: { type: "boolean", default: false, description: "Check for pattern mismatches" },
    severityLevel: {
      type: "string",
      default: "warning",
      enum: ["error", "warning", "info"],
      description: "Default severity level",
    },
    maxIssues: {
      type: "integer",
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
  "x-component": { id: "search-replace", type: "tool", category: "transform" },
  properties: {
    search: { type: "string", description: "Search pattern or literal text" },
    replace: { type: "string", description: "Replacement text" },
    regEx: { type: "boolean", default: false, description: "Treat search as a regular expression" },
    target: { type: "boolean", default: true, description: "Apply to target text" },
    source: { type: "boolean", default: false, description: "Apply to source text" },
    dotAll: { type: "boolean", default: false, description: "Dot matches newlines in regex" },
    caseInsensitive: { type: "boolean", default: false, description: "Case-insensitive matching" },
  },
};

// ── New: schemas exercising object/array/widget types ──

const nestedObjectSchema: ComponentSchema = {
  title: "HTML Filter",
  type: "object",
  "x-component": { id: "html-filter", type: "format", category: "convert" },
  "x-groups": [
    { id: "parser", label: "Parser Settings", fields: ["parser"] },
    { id: "extraction", label: "Extraction", fields: ["elements", "attributes"] },
    { id: "inline", label: "Inline Codes", fields: ["inlineCodes"] },
  ],
  properties: {
    parser: {
      type: "object",
      description: "Parser behavior settings",
      properties: {
        assumeWellformed: {
          type: "boolean",
          default: false,
          description:
            "Assume input HTML is well-formed XML. Faster but may fail on non-conforming HTML.",
        },
        preserveWhitespace: {
          type: "boolean",
          default: false,
          description: "Preserve original whitespace in extracted text",
        },
      },
    },
    elements: {
      type: "object",
      description: "Element extraction rules",
      "x-widget": "elementRulesEditor",
      additionalProperties: {
        type: "object",
        properties: {
          ruleTypes: {
            type: "array",
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
      description: "Global attribute extraction rules",
      "x-widget": "attributeRulesEditor",
      additionalProperties: {
        type: "object",
        properties: {
          ruleTypes: {
            type: "array",
            items: {
              type: "string",
              enum: ["ATTRIBUTE_TRANS", "ATTRIBUTE_WRITABLE", "ATTRIBUTE_READONLY", "ATTRIBUTE_ID"],
            },
          },
          allElementsExcept: {
            type: "array",
            items: { type: "string" },
          },
        },
      } as unknown as boolean,
    },
    inlineCodes: {
      type: "object",
      description: "Inline code detection and handling",
      "x-widget": "codeFinderRules",
      "x-presets": {
        "HTML Tags": { rules: [{ pattern: "</?\\w[^>]*>" }] },
        "Printf Placeholders": { rules: [{ pattern: "%[\\w.]*[dsfx]" }] },
      },
    },
    editorTitle: { type: "string", description: "Display title in editor" },
    simplifierRules: {
      type: "string",
      "x-widget": "simplifierRulesEditor",
      description: "Rules for simplifying inline code representation",
    },
  },
};

const arraySchema: ComponentSchema = {
  title: "Fixed-Width Columns",
  type: "object",
  "x-component": { id: "fixed-width", type: "format", category: "convert" },
  properties: {
    columns: {
      type: "array",
      description: "Column definitions for fixed-width parsing",
      items: {
        type: "object",
        properties: {
          name: { type: "string", description: "Column name" },
          start: { type: "integer", minimum: 0, description: "Start position (0-based)" },
          width: { type: "integer", minimum: 1, description: "Column width" },
          translatable: {
            type: "boolean",
            default: false,
            description: "Whether to extract for translation",
          },
        },
      },
    },
    encoding: {
      type: "string",
      default: "UTF-8",
      enum: ["UTF-8", "UTF-16", "ISO-8859-1", "Windows-1252"],
      description: "File character encoding",
    },
    skipLines: { type: "integer", default: 0, minimum: 0, description: "Header lines to skip" },
  },
};

const simpleArraySchema: ComponentSchema = {
  title: "Regex Filter",
  type: "object",
  "x-component": { id: "regex-filter", type: "format", category: "convert" },
  properties: {
    patterns: {
      type: "array",
      description: "Extraction patterns (regex)",
      items: { type: "string" },
    },
    caseSensitive: { type: "boolean", default: true },
  },
};

const jsonFallbackSchema: ComponentSchema = {
  title: "Custom Config",
  type: "object",
  "x-component": { id: "custom", type: "tool", category: "pipeline" },
  properties: {
    name: { type: "string", description: "Configuration name" },
    settings: {
      type: "object",
      description: "Arbitrary settings (JSON)",
    },
    tags: {
      type: "array",
      description: "Tags for this configuration",
    },
  },
};

const showIfSchema: ComponentSchema = {
  title: "Conditional Fields",
  type: "object",
  "x-component": { id: "conditional", type: "tool", category: "transform" },
  properties: {
    mode: {
      type: "string",
      default: "simple",
      enum: ["simple", "advanced"],
      description: "Processing mode",
    },
    threshold: {
      type: "number",
      default: 0.8,
      description: "Match threshold (only in advanced mode)",
      "x-showIf": { field: "mode", value: "advanced" },
    },
    maxResults: {
      type: "integer",
      default: 100,
      description: "Maximum results (only in advanced mode)",
      "x-showIf": { field: "mode", value: "advanced" },
    },
    caseSensitive: {
      type: "boolean",
      default: false,
      description: "Case-sensitive matching",
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
  name: "x-showIf Conditional Visibility",
  args: {
    schema: showIfSchema,
    initialValues: { mode: "simple" },
  },
};

export const ConditionalFieldsAdvanced: Story = {
  name: "x-showIf (advanced mode)",
  args: {
    schema: showIfSchema,
    initialValues: { mode: "advanced", threshold: 0.75, maxResults: 50 },
  },
};
