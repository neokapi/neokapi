/**
 * Schema Language: Groups and Layout
 *
 * Demonstrates x-groups for organizing parameters into collapsible sections,
 * and how group configuration affects the rendered form layout.
 */
import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { SchemaForm } from "@neokapi/flow-editor";
import type { ComponentSchema } from "@neokapi/flow-editor";

function SchemaStory({ schema, description }: { schema: ComponentSchema; description?: string }) {
  const [values, setValues] = useState<Record<string, unknown>>({});
  return (
    <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 24, maxWidth: 900 }}>
      <div>
        {description && <p className="text-sm text-muted-foreground mb-4">{description}</p>}
        <SchemaForm schema={schema} values={values} onChange={setValues} />
      </div>
      <div>
        <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">Schema</h4>
        <pre className="rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-80">
          {JSON.stringify(schema, null, 2)}
        </pre>
      </div>
    </div>
  );
}

const meta: Meta<typeof SchemaStory> = {
  title: "Schema Language/Groups & Layout",
  component: SchemaStory,
  tags: ["autodocs"],
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof SchemaStory>;

export const BasicGroups: Story = {
  name: "x-groups — Parameter Grouping",
  args: {
    description:
      'The `x-groups` array organizes properties into labeled, collapsible sections. Each group has an `id`, `label`, and a `fields` array referencing property names. Groups with 4 or fewer fields stay open by default.',
    schema: {
      title: "Format with Groups",
      type: "object",
      "ui:groups": [
        { id: "extraction", label: "Extraction", fields: ["extractAll", "exceptions"] },
        { id: "output", label: "Output", fields: ["encoding", "lineBreak", "bom"] },
      ],
      properties: {
        extractAll: { type: "boolean", title: "Extract All", description: "Extract all translatable strings", default: true },
        exceptions: { type: "string", title: "Exceptions", description: "Regex for keys to exclude" },
        encoding: { type: "string", title: "Output Encoding", default: "UTF-8", enum: ["UTF-8", "UTF-16", "ISO-8859-1"] },
        lineBreak: { type: "string", title: "Line Break", default: "platform", enum: ["platform", "lf", "crlf"] },
        bom: { type: "boolean", title: "Write BOM", description: "Write byte-order mark", default: false },
      },
    },
  },
};

export const CollapsedGroups: Story = {
  name: "collapsed — Auto-collapse Large Groups",
  args: {
    description:
      'Groups with 5+ fields are collapsible. The `collapsed: true` flag starts a group collapsed. By default, the first 2 groups are open and the rest are collapsed.',
    schema: {
      title: "Format with Many Groups",
      type: "object",
      "ui:groups": [
        {
          id: "basic",
          label: "Basic Settings",
          fields: ["extractAll", "encoding"],
        },
        {
          id: "advanced",
          label: "Advanced Settings",
          description: "Fine-tune extraction behavior",
          collapsed: true,
          fields: ["useCodeFinder", "codeFinderRules", "subfilter", "preserveWhitespace", "segmentation", "inlineTags"],
        },
      ],
      properties: {
        extractAll: { type: "boolean", title: "Extract All", default: true },
        encoding: { type: "string", title: "Encoding", default: "UTF-8" },
        useCodeFinder: { type: "boolean", title: "Use Code Finder", default: false },
        codeFinderRules: { type: "string", title: "Code Finder Rules", description: "Regex for inline codes" },
        subfilter: { type: "string", title: "Sub-filter", description: "Apply a secondary filter to extracted content" },
        preserveWhitespace: { type: "boolean", title: "Preserve Whitespace", default: false },
        segmentation: { type: "string", title: "Segmentation", default: "sentence", enum: ["sentence", "paragraph", "none"] },
        inlineTags: { type: "string", title: "Inline Tags", description: "Comma-separated list of inline element names" },
      },
    },
  },
};

export const GroupDescription: Story = {
  name: "description — Group-Level Help Text",
  args: {
    description: "Groups can have a `description` field that appears as help text below the group header.",
    schema: {
      title: "Groups with Descriptions",
      type: "object",
      "ui:groups": [
        {
          id: "extraction",
          label: "Extraction Rules",
          description: "Control which content is extracted for translation. Rules are evaluated in order — the first matching rule wins.",
          fields: ["extractAll", "pathRules", "exceptions"],
        },
        {
          id: "codeFinder",
          label: "Inline Code Detection",
          description: "Patterns that identify inline codes (placeholders, variables) within translatable text. Matched content is protected from translation.",
          fields: ["useCodeFinder", "codeFinderRules"],
        },
      ],
      properties: {
        extractAll: { type: "boolean", title: "Extract All Pairs", default: true },
        pathRules: { type: "string", title: "Path Rules", description: "JSONPath expressions for selective extraction" },
        exceptions: { type: "string", title: "Exceptions", description: "Regex for keys to exclude" },
        useCodeFinder: { type: "boolean", title: "Enable Code Finder", default: false },
        codeFinderRules: { type: "string", title: "Rules", description: "Regex patterns (one per line)", "ui:widget": "textarea" },
      },
    },
  },
};

export const UngroupedFields: Story = {
  name: "Ungrouped — Fields Without a Group",
  args: {
    description:
      "Properties not assigned to any group appear in an 'Other' section at the bottom. This handles schemas that only partially use groups.",
    schema: {
      title: "Partial Grouping",
      type: "object",
      "ui:groups": [
        { id: "main", label: "Main Settings", fields: ["extractAll"] },
      ],
      properties: {
        extractAll: { type: "boolean", title: "Extract All", default: true },
        encoding: { type: "string", title: "Encoding", default: "UTF-8", description: "This field has no group" },
        debug: { type: "boolean", title: "Debug Mode", description: "This field also has no group", default: false },
      },
    },
  },
};
