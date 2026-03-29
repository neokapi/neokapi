import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { FilterConfigEditor } from "../../components/filter/FilterConfigEditor";
import type { ComponentSchema } from "../../components/filter/types";

// ── Schemas ────────────────────────────────────────────────────

const pseudoTranslateSchema: ComponentSchema = {
  $id: "pseudo-translate",
  title: "Pseudo Translate",
  description: "Generate pseudo-translations for testing",
  type: "object",
  "x-component": {
    id: "pseudo-translate",
    type: "tool",
    category: "transform",
    displayName: "Pseudo Translate",
  },
  properties: {
    expansionPercent: {
      type: "integer",
      description: "Text expansion percentage (0 = none)",
      default: 0,
    },
    prefix: {
      type: "string",
      description: "Prefix to wrap pseudo text with",
      default: "[",
    },
    suffix: {
      type: "string",
      description: "Suffix to wrap pseudo text with",
      default: "]",
    },
    targetLocale: {
      type: "string",
      description: "Target locale for pseudo-translation",
    },
  },
};

const searchReplaceSchema: ComponentSchema = {
  $id: "search-and-replace",
  title: "Search and Replace",
  type: "object",
  "x-component": { id: "search-and-replace", type: "tool" },
  properties: {
    regEx: {
      type: "boolean",
      description: "Use regular expressions",
      default: false,
    },
    ignoreCase: {
      type: "boolean",
      description: "Case insensitive matching",
      default: false,
    },
    dotAll: {
      type: "boolean",
      description: "Dot matches all characters",
      default: false,
    },
    source: {
      type: "boolean",
      description: "Apply to source text",
      default: false,
    },
    target: {
      type: "boolean",
      description: "Apply to target text",
      default: true,
    },
    search: { type: "string", description: "Search pattern" },
    replace: { type: "string", description: "Replacement text" },
  },
};

const groupedSchema: ComponentSchema = {
  $id: "quality-check",
  title: "Quality Check",
  type: "object",
  "x-component": { id: "quality-check", type: "tool", category: "validate" },
  "x-groups": [
    {
      id: "whitespace",
      label: "Whitespace Checks",
      fields: ["checkLeadingWhitespace", "checkTrailingWhitespace", "checkDoubleSpaces"],
    },
    {
      id: "content",
      label: "Content Checks",
      fields: ["checkEmptyTarget", "checkTargetSameAsSource"],
    },
  ],
  properties: {
    checkLeadingWhitespace: {
      type: "boolean",
      description: "Check for leading whitespace",
      default: true,
    },
    checkTrailingWhitespace: {
      type: "boolean",
      description: "Check for trailing whitespace",
      default: true,
    },
    checkDoubleSpaces: {
      type: "boolean",
      description: "Check for double spaces",
      default: true,
    },
    checkEmptyTarget: {
      type: "boolean",
      description: "Check for empty translations",
      default: true,
    },
    checkTargetSameAsSource: {
      type: "boolean",
      description: "Check if target equals source",
      default: true,
    },
    targetLocale: { type: "string", description: "Target locale" },
  },
};

// ── Wrapper ────────────────────────────────────────────────────

function SchemaEditorWrapper({ schema }: { schema: ComponentSchema }) {
  const [value, setValue] = useState<Record<string, unknown>>({});
  return <FilterConfigEditor schema={schema} value={value} onChange={setValue} />;
}

// ── Meta ───────────────────────────────────────────────────────

const meta: Meta<typeof FilterConfigEditor> = {
  title: "Filter/SchemaConfigEditor",
  component: FilterConfigEditor,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "Dynamic parameter editor for tool schemas. Renders form controls based on JSON Schema with x-groups and x-widget extensions. Also exported as SchemaConfigEditor.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof FilterConfigEditor>;

// ── Stories ────────────────────────────────────────────────────

/** Simple tool with string and integer parameters. */
export const PseudoTranslate: Story = {
  render: () => <SchemaEditorWrapper schema={pseudoTranslateSchema} />,
};

/** Tool with a mix of boolean flags and text fields. */
export const SearchAndReplace: Story = {
  render: () => <SchemaEditorWrapper schema={searchReplaceSchema} />,
};

/** Tool schema using x-groups to organize parameters into collapsible sections. */
export const WithGroups: Story = {
  render: () => <SchemaEditorWrapper schema={groupedSchema} />,
};
