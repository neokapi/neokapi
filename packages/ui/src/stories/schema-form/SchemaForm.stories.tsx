import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { SchemaForm } from "../../components/schema-form";
import type { ComponentSchema } from "../../components/schema-form/types";

function Wrapper({
  schema,
  initialValues = {},
  compact = false,
}: {
  schema: ComponentSchema;
  initialValues?: Record<string, unknown>;
  compact?: boolean;
}) {
  const [values, setValues] = useState<Record<string, unknown>>(initialValues);
  const hasProps = schema.properties && Object.keys(schema.properties).length > 0;
  return (
    <div className="grid grid-cols-[1fr_1fr] gap-6 max-w-[1100px]">
      <div>
        <SchemaForm schema={schema} values={values} onChange={setValues} compact={compact} />
      </div>
      <div className="min-w-0">
        <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
          Schema
        </h4>
        <pre className="rounded-md bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-[600px]">
          {JSON.stringify(schema, null, 2)}
        </pre>
        {hasProps && (
          <>
            <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mt-4 mb-2">
              Values
            </h4>
            <pre className="rounded-md bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-[200px]">
              {JSON.stringify(values, null, 2)}
            </pre>
          </>
        )}
      </div>
    </div>
  );
}

// ── Schemas ───────────────────────────────────────────────────────────

const simpleSchema: ComponentSchema = {
  title: "Pseudo Translate",
  type: "object",
  toolMeta: { id: "pseudo-translate", category: "transform" },
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
  },
};

const groupedSchema: ComponentSchema = {
  title: "Quality Check",
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
      fields: ["checkEmptyTarget", "targetSameAsSource"],
    },
    {
      id: "output",
      label: "Output",
      fields: ["reportPath", "reportFormat"],
    },
  ],
  properties: {
    checkLeadingWhitespace: {
      type: "boolean",
      title: "Check Leading Whitespace",
      default: true,
      description: "Check for leading whitespace differences",
    },
    checkTrailingWhitespace: {
      type: "boolean",
      title: "Check Trailing Whitespace",
      default: true,
      description: "Check for trailing whitespace differences",
    },
    checkDoubleSpaces: {
      type: "boolean",
      title: "Check Double Spaces",
      default: false,
      description: "Check for double spaces",
    },
    checkEmptyTarget: {
      type: "boolean",
      title: "Warn on Empty Target",
      default: true,
      description: "Warn when target is empty",
    },
    targetSameAsSource: {
      type: "boolean",
      title: "Target Same as Source",
      default: true,
      description: "Warn when target equals source",
    },
    reportPath: {
      type: "string",
      title: "Report File Path",
      default: "${rootDir}/qa-report.html",
      "ui:widget": "path",
      description: "Output report file path",
    },
    reportFormat: {
      type: "string",
      title: "Report Format",
      options: [
        { value: "html", label: "HTML file" },
        { value: "tsv", label: "Tab-delimited file" },
        { value: "xml", label: "XML file" },
      ],
      default: "html",
      description: "Report output format",
    },
  },
};

const conditionalSchema: ComponentSchema = {
  title: "Batch Translation",
  type: "object",
  properties: {
    useTM: {
      type: "boolean",
      title: "Use Translation Memory",
      default: false,
      description: "Use translation memory for leveraging",
    },
    tmPath: {
      type: "string",
      title: "TM File Path",
      description: "Path to translation memory file",
      "ui:widget": "path",
      "ui:enabled": { field: "useTM", eq: true },
    },
    threshold: {
      type: "integer",
      title: "Match Threshold",
      default: 95,
      minimum: 0,
      maximum: 100,
      description: "Minimum match threshold",
      "ui:enabled": { field: "useTM", eq: true },
    },
    markAsMT: {
      type: "boolean",
      title: "Mark as Machine Translated",
      default: true,
      description: "Mark leveraged segments as machine translated",
      "ui:visible": { field: "useTM", eq: true },
    },
  },
};

const widgetsSchema: ComponentSchema = {
  title: "Widget Showcase",
  type: "object",
  properties: {
    textField: { type: "string", title: "Text Input", description: "Simple text field" },
    password: { type: "string", title: "Password", "ui:widget": "password" },
    textarea: { type: "string", title: "Code", "ui:widget": "textarea" },
    regex: { type: "string", title: "Pattern", "ui:widget": "regex" },
    tags: { type: "string", title: "Tags", "ui:widget": "tags" },
    toggle: { type: "boolean", title: "Enable feature", default: false },
    count: { type: "integer", title: "Count", minimum: 0, maximum: 100, default: 10 },
    mode: {
      type: "string",
      title: "Mode",
      options: [
        { value: "fast", label: "Fast" },
        { value: "balanced", label: "Balanced" },
        { value: "thorough", label: "Thorough" },
      ],
      default: "balanced",
    },
    segmented: {
      type: "string",
      title: "Output Type",
      "ui:widget": "segmented",
      enum: ["source", "target", "both"],
    },
    codeFinder: {
      type: "object",
      title: "Inline Codes",
      "ui:widget": "code-finder",
      "ui:presets": {
        "HTML Tags": { rules: [{ pattern: "</?\\w[^>]*>" }], sample: "<b>Bold</b>" },
        Printf: { rules: [{ pattern: "%[ds]" }], sample: "Found %d items" },
      },
    },
  },
};

const arraySchema: ComponentSchema = {
  title: "Array & List Editors",
  type: "object",
  properties: {
    tags: {
      type: "array",
      title: "Simple Tags",
      description: "String array rendered as pill chips",
      items: { type: "string" },
    },
    patterns: {
      type: "array",
      title: "Extraction Patterns",
      description: "Structured array with per-item fields",
      items: {
        type: "object",
        properties: {
          pattern: { type: "string", title: "Regex Pattern" },
          enabled: { type: "boolean", title: "Enabled", default: true },
        },
      },
    },
    extensions: {
      type: "array",
      title: "File Extensions",
      description: "Simple string list",
      items: { type: "string" },
    },
  },
};

const mapSchema: ComponentSchema = {
  title: "Map & Object Editors",
  type: "object",
  properties: {
    variables: {
      type: "object",
      title: "Environment Variables",
      description: "Key-value pairs — simple string values",
      additionalProperties: { type: "string" },
    },
    elementRules: {
      type: "object",
      title: "Element Rules",
      description: "Complex map — each entry has structured properties",
      "ui:widget": "element-rules",
      additionalProperties: {
        type: "object",
        properties: {
          ruleType: {
            type: "string",
            title: "Rule Type",
            enum: ["INLINE", "TEXTUNIT", "EXCLUDE"],
          },
          translatable: { type: "boolean", title: "Translatable", default: true },
        },
      },
    },
    settings: {
      type: "object",
      title: "Raw JSON Object",
      description: "Untyped object — rendered as JSON editor",
    },
  },
};

const nestedSchema: ComponentSchema = {
  title: "Nested Object Editor",
  type: "object",
  properties: {
    parser: {
      type: "object",
      title: "Parser Settings",
      description: "Nested object rendered inline at depth 0",
      properties: {
        encoding: { type: "string", title: "Encoding", default: "UTF-8" },
        strict: { type: "boolean", title: "Strict Mode", default: false },
        whitespace: {
          type: "object",
          title: "Whitespace Handling",
          description: "Deeper nesting — rendered as drill-down at depth 2+",
          properties: {
            preserve: { type: "boolean", title: "Preserve Whitespace", default: false },
            normalize: { type: "boolean", title: "Normalize Spaces", default: true },
            trimLines: { type: "boolean", title: "Trim Lines", default: false },
          },
        },
      },
    },
  },
};

// ── Meta ──────────────────────────────────────────────────────────────

const meta: Meta<typeof SchemaForm> = {
  title: "Formats & Tools/Schema/SchemaForm",
  component: SchemaForm,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "Schema-driven configuration form. Auto-generates form fields from a ComponentSchema, supporting groups, conditional visibility/enablement, preset comparison, and 15+ widget types.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof SchemaForm>;

// ── Stories ───────────────────────────────────────────────────────────

export const Simple: Story = {
  render: () => <Wrapper schema={simpleSchema} />,
};

export const WithGroups: Story = {
  render: () => <Wrapper schema={groupedSchema} />,
};

export const ConditionalVisibility: Story = {
  render: () => <Wrapper schema={conditionalSchema} />,
};

export const AllWidgets: Story = {
  render: () => <Wrapper schema={widgetsSchema} />,
};

export const Compact: Story = {
  render: () => <Wrapper schema={simpleSchema} compact />,
};

export const ArrayEditors: Story = {
  name: "Array & List Editors",
  render: () => (
    <Wrapper
      schema={arraySchema}
      initialValues={{
        tags: ["localization", "i18n", "okapi"],
        patterns: [
          { pattern: "</?\\w[^>]*>", enabled: true },
          { pattern: "\\{\\d+\\}", enabled: false },
        ],
        extensions: [".html", ".htm"],
      }}
    />
  ),
};

export const MapEditors: Story = {
  name: "Map & Object Editors",
  render: () => (
    <Wrapper
      schema={mapSchema}
      initialValues={{
        variables: {
          ROOT_DIR: "/projects/demo",
          OUTPUT_DIR: "/output",
          LANG: "fr-FR",
        },
        elementRules: {
          div: { ruleType: "TEXTUNIT", translatable: true },
          span: { ruleType: "INLINE", translatable: true },
          script: { ruleType: "EXCLUDE", translatable: false },
        },
        settings: { debug: true, version: 2 },
      }}
    />
  ),
};

export const NestedObjects: Story = {
  name: "Nested Object Editor",
  render: () => (
    <Wrapper
      schema={nestedSchema}
      initialValues={{
        parser: {
          encoding: "UTF-8",
          strict: false,
          whitespace: { preserve: false, normalize: true, trimLines: false },
        },
      }}
    />
  ),
};
