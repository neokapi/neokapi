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
    expansionPercent: { type: "integer", default: 30, minimum: 0, maximum: 200, description: "Expand text length by percentage" },
    applyAccents: { type: "boolean", default: true, description: "Apply diacritical marks to simulate translated text" },
    padWithX: { type: "boolean", default: false, description: "Pad expansion with 'x' characters instead of spaces" },
  },
};

const qaCheckSchema: ComponentSchema = {
  title: "QA Check",
  type: "object",
  "x-component": { id: "qa-check", type: "tool", category: "validate" },
  "x-groups": [
    { id: "whitespace", label: "Whitespace Checks", fields: ["checkLeadingWhitespace", "checkTrailingWhitespace", "checkDoubleSpaces"] },
    { id: "content", label: "Content Checks", fields: ["checkMissingTranslation", "checkInlineCodes", "checkPatterns"] },
  ],
  properties: {
    checkLeadingWhitespace: { type: "boolean", default: true, description: "Check for leading whitespace mismatches" },
    checkTrailingWhitespace: { type: "boolean", default: true, description: "Check trailing whitespace" },
    checkDoubleSpaces: { type: "boolean", default: true, description: "Flag double spaces in target" },
    checkMissingTranslation: { type: "boolean", default: true, description: "Flag empty translations" },
    checkInlineCodes: { type: "boolean", default: true, description: "Verify inline codes are preserved" },
    checkPatterns: { type: "boolean", default: false, description: "Check for pattern mismatches" },
    severityLevel: { type: "string", default: "warning", enum: ["error", "warning", "info"], description: "Default severity level" },
    maxIssues: { type: "integer", default: 100, minimum: 1, maximum: 10000, description: "Maximum issues to report" },
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
