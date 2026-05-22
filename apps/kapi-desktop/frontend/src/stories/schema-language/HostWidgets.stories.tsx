/**
 * Schema Language: Host-backed Widgets
 *
 * The shared SchemaForm exposes host-injectable capabilities for fields that
 * need a filesystem or credential store: the `path` / `folder-picker` widgets
 * (file & folder browsers) and the `credential-picker` widget. In Kapi Desktop
 * these are wired to native Wails dialogs and the OS-keychain provider vault via
 * a `SchemaFormHost`; here they run against a stubbed host so the Browse buttons
 * and credential dropdown are demonstrated without a Wails runtime.
 *
 * Without a host (the docs website, a bare Storybook) the same widgets degrade
 * to plain text inputs — see the "No Host (Degraded)" story.
 */
import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { SchemaForm, type ComponentSchema, type SchemaFormHost } from "@neokapi/ui-primitives";

// A stub host standing in for Kapi Desktop's Wails backend. Storybook has no
// filesystem, so onBrowse returns a canned path instead of opening a dialog.
const stubHost: SchemaFormHost = {
  onBrowse: async (request) => {
    if (request.kind === "directory") return "/Users/me/projects/site/locales";
    if (request.forSaveAs) return "/Users/me/projects/site/out/result.tmx";
    return "/Users/me/projects/site/input/document.html";
  },
  credentials: (resourceKind) => {
    const all = [
      { value: "anthropic-prod", label: "Anthropic (claude-sonnet)", kind: "anthropic" },
      { value: "openai-test", label: "OpenAI (gpt-4o)", kind: "openai" },
      { value: "ollama-local", label: "Ollama (llama3)", kind: "ollama" },
    ];
    const scoped = resourceKind ? all.filter((c) => c.kind === resourceKind) : all;
    return scoped.map(({ value, label }) => ({ value, label }));
  },
};

function HostStory({
  schema,
  description,
  host,
}: {
  schema: ComponentSchema;
  description?: string;
  host?: SchemaFormHost;
}) {
  const [values, setValues] = useState<Record<string, unknown>>({});
  return (
    <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 24, maxWidth: 900 }}>
      <div>
        {description && <p className="text-sm text-muted-foreground mb-4">{description}</p>}
        <SchemaForm schema={schema} values={values} onChange={setValues} host={host} />
      </div>
      <div style={{ minWidth: 0 }}>
        <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
          Values
        </h4>
        <pre className="rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-40">
          {JSON.stringify(values, null, 2)}
        </pre>
      </div>
    </div>
  );
}

const pathSchema: ComponentSchema = {
  title: "Path & Credential Widgets",
  type: "object",
  properties: {
    inputFile: {
      type: "string",
      title: "Input File",
      "ui:widget": "path",
      "x-path": {
        browseTitle: "Choose a document",
        accepts: ["html", "htm", "txt"],
        filters: [{ name: "HTML", extensions: "*.html *.htm" }],
      },
    },
    outputDir: {
      type: "string",
      title: "Output Folder",
      "ui:widget": "folder-picker",
      "x-path": { browseTitle: "Choose an output folder" },
    },
    exportTo: {
      type: "string",
      title: "Export To",
      "ui:widget": "path",
      "x-path": {
        browseTitle: "Save TMX as",
        forSaveAs: true,
        filters: [{ name: "TMX", extensions: "*.tmx" }],
      },
    },
    provider: {
      type: "string",
      title: "AI Provider",
      "ui:widget": "credential-picker",
      "x-path": { resourceKind: "anthropic" },
    },
    anyProvider: {
      type: "string",
      title: "Any Credential",
      "ui:widget": "credential-picker",
    },
  },
};

const meta: Meta<typeof HostStory> = {
  title: "Formats & Tools/Schema/Host Widgets",
  component: HostStory,
  tags: ["autodocs"],
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof HostStory>;

export const WithHost: Story = {
  name: "With Host — File / Folder / Credential Pickers",
  args: {
    description:
      "A stub host supplies onBrowse (file/folder/save-as dialogs) and credentials. The path fields show a Browse button; credential fields show a dropdown of saved providers (the AI Provider field is scoped to resourceKind: anthropic).",
    schema: pathSchema,
    host: stubHost,
  },
};

export const NoHostDegraded: Story = {
  name: "No Host (Degraded) — Plain Text Inputs",
  args: {
    description:
      "Without a host the path widgets drop the Browse button and the credential picker becomes a plain text input, so every field stays usable in hosts with no filesystem or credential store.",
    schema: pathSchema,
    host: undefined,
  },
};
