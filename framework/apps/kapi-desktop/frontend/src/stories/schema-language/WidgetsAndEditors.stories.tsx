/**
 * Schema Language: Widgets and Editors
 *
 * Demonstrates ui:widget and ui:widget-options for controlling how properties
 * are rendered beyond the default type-based dispatch.
 */
import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { SchemaForm } from "@neokapi/ui-primitives";
import type { ComponentSchema } from "@neokapi/ui-primitives";

function SchemaStory({ schema, description }: { schema: ComponentSchema; description?: string }) {
  const [values, setValues] = useState<Record<string, unknown>>({});
  return (
    <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 24, maxWidth: 900  }}>
      <div>
        {description && <p className="text-sm text-muted-foreground mb-4">{description}</p>}
        <SchemaForm schema={schema} values={values} onChange={setValues} />
      </div>
      <div style={{ minWidth: 0 }}>
        <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">Values</h4>
        <pre className="rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-40">
          {JSON.stringify(values, null, 2)}
        </pre>
        <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mt-4 mb-2">Schema</h4>
        <pre className="rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-60">
          {JSON.stringify(schema, null, 2)}
        </pre>
      </div>
    </div>
  );
}

const meta: Meta<typeof SchemaStory> = {
  title: "Formats & Tools/Schema/Widgets & Editors",
  component: SchemaStory,
  tags: ["autodocs"],
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof SchemaStory>;

export const TextWidget: Story = {
  name: "ui:widget: text — Standard Text Input",
  args: {
    description: "The default for string properties. Supports `ui:placeholder` for hint text.",
    schema: {
      title: "Text Widgets",
      type: "object",
      properties: {
        name: { type: "string", title: "Name", "ui:widget": "text" },
        description: { type: "string", title: "Description", "ui:widget": "text", "ui:placeholder": "Enter a description..." },
      },
    },
  },
};

export const TextareaWidget: Story = {
  name: "ui:widget: textarea — Multiline Text",
  args: {
    description: 'Renders a multiline text area. The canonical widget name is `"multilineText"`. The alias `"textarea"` is also accepted. Use for rules, patterns, or any long-form text input.',
    schema: {
      title: "Textarea Widget",
      type: "object",
      properties: {
        rules: {
          type: "string",
          title: "Extraction Rules (multilineText)",
          description: "Using ui:widget: multilineText",
          "ui:widget": "multilineText",
        },
        notes: {
          type: "string",
          title: "Notes (textarea alias)",
          description: "Using ui:widget: textarea — normalized to multilineText",
          "ui:widget": "textarea",
        },
      },
    },
  },
};

export const RegexBuilderWidget: Story = {
  name: "ui:widget: regexBuilder — Regex Pattern Input",
  args: {
    description: "A text input styled for regex patterns. Provides visual feedback for regex syntax.",
    schema: {
      title: "Regex Builder",
      type: "object",
      properties: {
        pattern: {
          type: "string",
          title: "Extraction Pattern",
          description: "Regex for matching translatable content",
          "ui:widget": "regexBuilder",
        },
      },
    },
  },
};

export const TagListWidget: Story = {
  name: "ui:widget: tagList — Tag/Token Input",
  args: {
    description: "A tag list editor for entering multiple values as visual tags.",
    schema: {
      title: "Tag List",
      type: "object",
      properties: {
        inlineTags: {
          type: "string",
          title: "Inline Element Names",
          description: "HTML elements to treat as inline (e.g., b, i, span)",
          "ui:widget": "tagList",
        },
      },
    },
  },
};

export const PasswordWidget: Story = {
  name: "ui:widget: text (password) — Masked Input",
  args: {
    description: "The ui:widget-options metadata can specify `text.password: true` for sensitive fields.",
    schema: {
      title: "Password Field",
      type: "object",
      properties: {
        apiKey: {
          type: "string",
          title: "API Key",
          description: "Your authentication token",
          "ui:widget": "password",
        },
      },
    },
  },
};

export const SpinWidget: Story = {
  name: "ui:widget: spin — Numeric Spinner",
  args: {
    description: "Numeric input with increment/decrement controls from ui:widget-options.",
    schema: {
      title: "Spin Widget",
      type: "object",
      properties: {
        maxWidth: {
          type: "integer",
          title: "Max Width",
          description: "Maximum line width in characters",
          default: 80,
          minimum: 20,
          maximum: 500,
          "ui:widget": "spin",
        },
      },
    },
  },
};

export const CheckListWidget: Story = {
  name: "ui:widget: checkList — Named Checkbox List",
  args: {
    description: "Renders a list of named checkboxes from the ui:widget-options.entries array. Each entry has a name, title, and optional description.",
    schema: {
      title: "Check List",
      type: "object",
      properties: {
        features: {
          type: "string",
          title: "Enabled Features",
          description: "Select which features to enable",
          "ui:widget": "checkList",
          "ui:widget-options": {
            entries: [
              { name: "segmentation", title: "Segmentation", description: "Split text into sentences" },
              { name: "codeFinder", title: "Code Finder", description: "Detect inline codes" },
              { name: "subfilter", title: "Sub-filtering", description: "Apply secondary filter to embedded content" },
            ],
          },
        },
      },
    },
  },
};

export const DropdownWidget: Story = {
  name: "ui:widget: dropdown — Dropdown Select",
  args: {
    description: "Dropdown selection from ui:widget-options. Maps to the same rendering as enum + ui:enum-labels.",
    schema: {
      title: "Dropdown Widget",
      type: "object",
      properties: {
        lineBreak: {
          type: "string",
          title: "Line Break Style",
          enum: ["lf", "crlf", "platform"],
          default: "platform",
          "ui:widget": "dropdown",
          "ui:enum-labels": { lf: "Unix (LF)", crlf: "Windows (CRLF)", platform: "Platform Default" },
        },
      },
    },
  },
};
