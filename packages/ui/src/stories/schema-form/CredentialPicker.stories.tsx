import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { SchemaForm } from "../../components/schema-form";
import type { ComponentSchema } from "../../components/schema-form/types";

function Wrapper({
  schema,
  initialValues = {},
}: {
  schema: ComponentSchema;
  initialValues?: Record<string, unknown>;
}) {
  const [values, setValues] = useState<Record<string, unknown>>(initialValues);
  return (
    <div className="grid grid-cols-[1fr_1fr] gap-6 max-w-[1100px]">
      <div>
        <SchemaForm schema={schema} values={values} onChange={setValues} compact />
      </div>
      <div className="min-w-0">
        <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
          Values
        </h4>
        <pre className="rounded-md bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-[200px]">
          {JSON.stringify(values, null, 2)}
        </pre>
        <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mt-4 mb-2">
          Schema
        </h4>
        <pre className="rounded-md bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-[400px]">
          {JSON.stringify(schema, null, 2)}
        </pre>
      </div>
    </div>
  );
}

// ── Schemas ───────────────────────────────────────────────────────────

// Simulates the schema injected by the desktop backend for an AI tool
// that requires credentials. The "credential" field is added at runtime
// by injectCredentialPicker(), and manual fields get ui:visible conditions.
const aiTranslateWithCredentials: ComponentSchema = {
  title: "AI Translate",
  description: "Translate content using an LLM provider",
  type: "object",
  toolMeta: {
    id: "translate",
    category: "translate",
    requires: ["target-language", "credentials"],
  },
  "ui:groups": [
    {
      id: "provider",
      label: "Provider",
      fields: ["credential", "provider", "apiKey", "model"],
    },
    {
      id: "other",
      label: "Other",
      fields: ["batchConcurrency", "batchSize", "skipMatched"],
    },
  ],
  properties: {
    credential: {
      type: "string",
      title: "Credential",
      description: "Saved credential to use for this tool",
      default: "",
      options: [
        { value: "", label: "Custom (manual entry)" },
        { value: "my-anthropic", label: "My Anthropic (claude-sonnet-4-5-20250514)" },
        { value: "work-openai", label: "Work OpenAI (gpt-4o)" },
        { value: "local-ollama", label: "Local Ollama" },
      ],
      "ui:widget": "credential-picker",
      "ui:order": -1,
    },
    provider: {
      type: "string",
      title: "AI Provider",
      description: "AI provider",
      default: "anthropic",
      options: [
        { value: "anthropic", label: "anthropic" },
        { value: "openai", label: "openai" },
        { value: "gemini", label: "gemini" },
        { value: "ollama", label: "ollama" },
      ],
      "ui:visible": { field: "credential", eq: "" },
    },
    apiKey: {
      type: "string",
      title: "API Key",
      description: "API key for the AI provider",
      "ui:visible": { field: "credential", eq: "" },
    },
    model: {
      type: "string",
      title: "Model",
      description: "AI model name",
      "ui:visible": { field: "credential", eq: "" },
    },
    batchConcurrency: {
      type: "integer",
      title: "Batch Concurrency",
      default: 1,
      minimum: 1,
    },
    batchSize: {
      type: "integer",
      title: "Batch Size",
      default: 100,
      minimum: 1,
    },
    skipMatched: {
      type: "boolean",
      title: "Skip Matched",
      description: "Skip blocks that already have a target translation",
    },
  },
};

const singleCredential: ComponentSchema = {
  ...aiTranslateWithCredentials,
  title: "AI Translate (single credential)",
  properties: {
    ...aiTranslateWithCredentials.properties,
    credential: {
      ...aiTranslateWithCredentials.properties!.credential,
      options: [
        { value: "", label: "Custom (manual entry)" },
        { value: "my-key", label: "My Anthropic Key (claude-sonnet-4-5-20250514)" },
      ],
    },
  },
};

const noCredentials: ComponentSchema = {
  ...aiTranslateWithCredentials,
  title: "AI Translate (no saved credentials)",
  properties: {
    ...aiTranslateWithCredentials.properties,
    credential: {
      ...aiTranslateWithCredentials.properties!.credential,
      options: [{ value: "", label: "Custom (manual entry)" }],
    },
  },
};

// ── Meta ─────────────────────────────────────────────────────────────

const meta: Meta<typeof SchemaForm> = {
  title: "Formats & Tools/Schema/Credential Picker",
  component: SchemaForm,
  parameters: {
    layout: "padded",
    docs: {
      description: {
        component:
          "Demonstrates the credential picker widget injected by the desktop backend for AI tools. " +
          "When a saved credential is selected, manual provider/apiKey/model fields are hidden via conditional visibility. " +
          'Selecting "Custom (manual entry)" reveals the manual fields.',
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof SchemaForm>;

// ── Stories ───────────────────────────────────────────────────────────

export const MultipleCredentials: Story = {
  name: "Multiple saved credentials",
  render: () => <Wrapper schema={aiTranslateWithCredentials} />,
};

export const CredentialSelected: Story = {
  name: "With credential pre-selected",
  render: () => (
    <Wrapper schema={aiTranslateWithCredentials} initialValues={{ credential: "my-anthropic" }} />
  ),
};

export const CustomManualEntry: Story = {
  name: "Custom (manual entry)",
  render: () => (
    <Wrapper
      schema={aiTranslateWithCredentials}
      initialValues={{ credential: "", provider: "openai", apiKey: "sk-test-key", model: "gpt-4o" }}
    />
  ),
};

export const SingleCredential: Story = {
  name: "Single saved credential",
  render: () => <Wrapper schema={singleCredential} />,
};

export const NoSavedCredentials: Story = {
  name: "No saved credentials",
  render: () => <Wrapper schema={noCredentials} />,
};
