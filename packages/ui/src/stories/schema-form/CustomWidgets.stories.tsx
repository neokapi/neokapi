import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { SchemaForm } from "../../components/schema-form";
import type { ComponentSchema } from "../../components/schema-form/types";
import type { SchemaFormHost, SchemaFormBrowseRequest } from "../../components/schema-form/host";

// Side-by-side wrapper: live form on the left, assembled values on the right.
// The custom widgets all assemble into the same flat values object SchemaForm
// produces for every other field, so this view doubles as a value-shape probe.
function Wrapper({
  schema,
  initialValues = {},
  host,
}: {
  schema: ComponentSchema;
  initialValues?: Record<string, unknown>;
  host?: SchemaFormHost;
}) {
  const [values, setValues] = useState<Record<string, unknown>>(initialValues);
  return (
    <div className="grid grid-cols-[1fr_1fr] gap-6 max-w-[1100px]">
      <div>
        <SchemaForm schema={schema} values={values} onChange={setValues} host={host} />
      </div>
      <div className="min-w-0">
        <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
          Values
        </h4>
        <pre className="rounded-md bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-[280px]">
          {JSON.stringify(values, null, 2)}
        </pre>
        <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mt-4 mb-2">
          Schema
        </h4>
        <pre className="rounded-md bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-[420px]">
          {JSON.stringify(schema, null, 2)}
        </pre>
      </div>
    </div>
  );
}

// A stub host that mimics what kapi-desktop's Wails backend injects: it
// resolves browse dialogs with canned paths and supplies a credential list.
// On the docs website no host is provided, so the same widgets degrade to
// plain text inputs.
const stubHost: SchemaFormHost = {
  onBrowse: async (req: SchemaFormBrowseRequest) => {
    if (req.kind === "directory") return "/Users/demo/projects/site";
    if (req.forSaveAs) return "/Users/demo/exports/output.tmx";
    return "/Users/demo/projects/site/source.html";
  },
  credentials: () => [
    { value: "", label: "Custom (manual entry)" },
    { value: "my-anthropic", label: "My Anthropic (claude)" },
    { value: "work-openai", label: "Work OpenAI (gpt-4o)" },
  ],
};

// ── Schemas ───────────────────────────────────────────────────────────

const numberListSchema: ComponentSchema = {
  title: "Number List",
  type: "object",
  properties: {
    columns: {
      type: "string",
      title: "Translatable Columns",
      "ui:widget": "number-list",
      description: "Comma or space separated column indices",
    },
  },
};

const pickerSchema: ComponentSchema = {
  title: "File & Folder Pickers",
  type: "object",
  properties: {
    inputFile: {
      type: "string",
      title: "Input File",
      "ui:widget": "file-picker",
      description: "Browse delegates to the host; degrades to a text input on the web",
      "ui:widget-options": {
        browseTitle: "Choose input",
        filters: [
          { name: "HTML (*.html *.htm)", extensions: "*.html *.htm" },
          { name: "All Files (*.*)", extensions: "*.*" },
        ],
      },
    },
    outputFile: {
      type: "string",
      title: "Output TMX",
      "ui:widget": "file-picker",
      "ui:widget-options": {
        browseTitle: "Save TMX as",
        forSaveAs: true,
        filters: [{ name: "TMX (*.tmx)", extensions: "*.tmx" }],
      },
    },
    workDir: {
      type: "string",
      title: "Working Directory",
      "ui:widget": "folder-picker",
      "ui:widget-options": { browseTitle: "Choose directory" },
    },
  },
};

const credentialSchema: ComponentSchema = {
  title: "Credential Picker (host-injected)",
  type: "object",
  properties: {
    credential: {
      type: "string",
      title: "Credential",
      "ui:widget": "credential-picker",
      description: "Populated from the host's credential vault",
    },
  },
};

const credentialInlineSchema: ComponentSchema = {
  title: "Credential Picker (inline options)",
  type: "object",
  properties: {
    credential: {
      type: "string",
      title: "Credential",
      "ui:widget": "credential-picker",
      description: "Options baked into the schema (kapi-desktop's injectCredentialPicker path)",
      options: [
        { value: "", label: "Custom (manual entry)" },
        { value: "my-anthropic", label: "My Anthropic (claude)" },
      ],
    },
  },
};

const ruleWidgetsSchema: ComponentSchema = {
  title: "Rule Editors",
  type: "object",
  properties: {
    elementRules: {
      type: "object",
      title: "Element Rules",
      "ui:widget": "element-rules",
      additionalProperties: {
        type: "object",
        properties: {
          ruleType: { type: "string", title: "Rule Type", enum: ["INLINE", "TEXTUNIT", "EXCLUDE"] },
          translatable: { type: "boolean", title: "Translatable", default: true },
        },
      },
    },
    attributeRules: {
      type: "object",
      title: "Attribute Rules",
      "ui:widget": "attribute-rules",
      additionalProperties: { type: "string" },
    },
    simplifierRules: {
      type: "string",
      title: "Simplifier Rules",
      "ui:widget": "simplifier-rules",
    },
  },
};

const inputWidgetsSchema: ComponentSchema = {
  title: "Inline Input Widgets",
  type: "object",
  properties: {
    pattern: { type: "string", title: "Regex Pattern", "ui:widget": "regex" },
    tags: { type: "string", title: "Tags", "ui:widget": "tags" },
    output: {
      type: "string",
      title: "Output",
      "ui:widget": "segmented",
      enum: ["source", "target", "both"],
    },
    mode: {
      type: "string",
      title: "Mode",
      "ui:widget": "select",
      options: [
        { value: "fast", label: "Fast" },
        { value: "balanced", label: "Balanced" },
        { value: "thorough", label: "Thorough" },
      ],
    },
    checks: {
      type: "object",
      title: "Checks",
      "ui:widget": "checklist",
      "ui:widget-options": {
        entries: [
          { name: "trim", title: "Trim whitespace", description: "Remove leading/trailing space" },
          { name: "dedupe", title: "De-duplicate entries" },
        ],
      },
    },
  },
};

const codeFinderSchema: ComponentSchema = {
  title: "Code Finder",
  type: "object",
  properties: {
    codes: {
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

// ── Meta ──────────────────────────────────────────────────────────────

const meta: Meta<typeof SchemaForm> = {
  title: "Formats & Tools/Schema/Custom Widgets",
  component: SchemaForm,
  parameters: {
    layout: "padded",
    docs: {
      description: {
        component:
          "Custom schema-form widgets rendered through SchemaForm. File/folder/credential " +
          "widgets accept host-injected capabilities (the `host` prop) and degrade to plain " +
          "text inputs when a host does not provide them.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof SchemaForm>;

// ── Stories ───────────────────────────────────────────────────────────

export const NumberList: Story = {
  render: () => <Wrapper schema={numberListSchema} initialValues={{ columns: "1, 2, 5" }} />,
};

export const NumberListInvalid: Story = {
  name: "Number List (invalid token)",
  render: () => <Wrapper schema={numberListSchema} initialValues={{ columns: "1, two, 3" }} />,
};

export const PickersWithHost: Story = {
  name: "File & Folder Pickers (host injected)",
  render: () => <Wrapper schema={pickerSchema} host={stubHost} />,
};

export const PickersNoHost: Story = {
  name: "File & Folder Pickers (web — no host)",
  render: () => <Wrapper schema={pickerSchema} />,
};

export const CredentialHostInjected: Story = {
  name: "Credential Picker (host injected)",
  render: () => <Wrapper schema={credentialSchema} host={stubHost} />,
};

export const CredentialInlineOptions: Story = {
  name: "Credential Picker (inline options)",
  render: () => <Wrapper schema={credentialInlineSchema} />,
};

export const CredentialNoSource: Story = {
  name: "Credential Picker (web — text fallback)",
  render: () => <Wrapper schema={credentialSchema} />,
};

export const RuleEditors: Story = {
  name: "Rule Editors (element / attribute / simplifier)",
  render: () => (
    <Wrapper
      schema={ruleWidgetsSchema}
      initialValues={{
        elementRules: {
          div: { ruleType: "TEXTUNIT", translatable: true },
          span: { ruleType: "INLINE", translatable: true },
        },
        attributeRules: { title: "TEXTUNIT", alt: "TEXTUNIT" },
        simplifierRules: 'if TYPE = "b";\nif TAG_TYPE = STANDALONE;',
      }}
    />
  ),
};

export const InlineInputs: Story = {
  name: "Regex / Tags / Segmented / Select / Checklist",
  render: () => (
    <Wrapper
      schema={inputWidgetsSchema}
      initialValues={{
        pattern: "\\{\\d+\\}",
        tags: "html, i18n, okapi",
        output: "both",
        mode: "balanced",
        checks: { trim: true, dedupe: false },
      }}
    />
  ),
};

export const CodeFinder: Story = {
  name: "Code Finder",
  render: () => (
    <Wrapper
      schema={codeFinderSchema}
      initialValues={{
        codes: { rules: [{ pattern: "%[ds]" }], sample: "Found %d of %d items" },
      }}
    />
  ),
};
