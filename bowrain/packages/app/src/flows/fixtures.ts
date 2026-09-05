// Sample flow definitions and tools in the shapes the server returns, plus an
// in-memory API over them, for the flows stories and tests.

import type {
  ApiAdapter,
  ComponentSchema,
  FlowDefinitionInfo,
  ProviderConfig,
  ToolInfo,
} from "@neokapi/ui";

export const sampleTools: ToolInfo[] = [
  {
    name: "recycle",
    display_name: "Recycle",
    description: "Reuse approved wording from the content memory.",
    category: "leverage",
  },
  {
    name: "translate",
    display_name: "Translate",
    description: "Translate with an AI provider.",
    category: "translate",
  },
  {
    name: "qa",
    display_name: "Quality Check",
    description: "Check the result against the rules.",
    category: "validate",
  },
  {
    name: "term-check",
    display_name: "Term check",
    description: "Hold the wording to the terms store.",
    category: "validate",
  },
  {
    name: "pseudo-translate",
    display_name: "Pseudo Translate",
    description: "Generate pseudo-translations for testing.",
    category: "translate",
  },
];

/**
 * Tool option schemas in the registry's shape, keyed by tool. A tool absent
 * here has none, and its step shows no options control.
 */
export const sampleSchemas: Record<string, ComponentSchema> = {
  translate: {
    title: "Translate",
    type: "object",
    properties: {
      credential: {
        type: "string",
        title: "Credential",
        description: "The provider configuration to translate with",
        "ui:widget": "credential-picker",
      },
      skipMatched: {
        type: "boolean",
        title: "Skip matched",
        description: "Leave the blocks the content memory already filled",
        default: false,
      },
    },
  },
  "term-check": {
    title: "Terminology Check",
    type: "object",
    properties: {
      caseSensitive: {
        type: "boolean",
        title: "Case Sensitive",
        description: "Whether term matching is case-sensitive",
        default: false,
      },
    },
  },
  "pseudo-translate": {
    title: "Pseudo Translate",
    type: "object",
    properties: {
      expansionPercent: {
        type: "integer",
        title: "Expansion Percent",
        description: "Extra padding percentage added to simulate translation expansion",
        default: 0,
        minimum: 0,
      },
      prefix: {
        type: "string",
        title: "Prefix",
        description: "Characters prepended before each translated block",
        default: "\u2592 ",
      },
      suffix: {
        type: "string",
        title: "Suffix",
        description: "Characters appended after each translated block",
        default: " \u2592",
      },
    },
  },
};

/** The workspace's saved provider configurations, as a credential picker offers them. */
export const sampleProviders: ProviderConfig[] = [
  {
    id: "prov-1",
    name: "claude",
    provider_type: "anthropic",
    model: "claude-sonnet-4-6",
    base_url: "",
  },
  {
    id: "prov-2",
    name: "local-llama",
    provider_type: "ollama",
    model: "llama3.1",
    base_url: "http://localhost:11434",
  },
];

/** The server's built-in translate flow: one row, chained by its edges. */
export const builtInTranslate: FlowDefinitionInfo = {
  id: "translate",
  name: "Translate",
  description: "Translate with guardrails: content memory reuse, then AI translate, then checks",
  source: "built-in",
  nodes: [
    {
      id: "recycle",
      type: "tool",
      name: "recycle",
      label: "Memory Reuse",
      position: { x: 0, y: 100 },
    },
    {
      id: "translate",
      type: "tool",
      name: "translate",
      label: "Translate",
      config: { skipMatched: true },
      position: { x: 250, y: 100 },
    },
    { id: "qa", type: "tool", name: "qa", label: "Quality Check", position: { x: 500, y: 100 } },
  ],
  edges: [
    { id: "e-recycle-translate", source: "recycle", target: "translate" },
    { id: "e-translate-qa", source: "translate", target: "qa" },
  ],
};

export const builtInPseudo: FlowDefinitionInfo = {
  id: "pseudo-translate",
  name: "Pseudo Translate",
  description: "Generate pseudo-translations for testing",
  source: "built-in",
  nodes: [
    {
      id: "pseudo-translate",
      type: "tool",
      name: "pseudo-translate",
      label: "Pseudo Translate",
      position: { x: 0, y: 100 },
    },
  ],
  edges: [],
};

/** A project flow with a fan-out, in the shape the editor persists. */
export const projectReview: FlowDefinitionInfo = {
  id: "flow-review",
  name: "Translate and review",
  description: "Translate, then run both checks at once",
  source: "project",
  nodes: [
    { id: "tool-0", type: "tool", name: "translate", position: { x: 200, y: 0 } },
    { id: "tool-1", type: "tool", name: "qa", position: { x: 60, y: 260 } },
    { id: "tool-2", type: "tool", name: "term-check", position: { x: 340, y: 260 } },
  ],
  edges: [
    { id: "e-tool-0-tool-1", source: "tool-0", target: "tool-1" },
    { id: "e-tool-0-tool-2", source: "tool-0", target: "tool-2" },
  ],
  created_at: "2026-09-01T09:00:00Z",
  modified_at: "2026-09-02T14:30:00Z",
};

export const sampleFlows: FlowDefinitionInfo[] = [builtInTranslate, builtInPseudo, projectReview];

export interface FlowsApiOptions {
  flows?: FlowDefinitionInfo[];
  tools?: ToolInfo[];
  /** Option schemas by tool; a tool absent here has none. */
  schemas?: Record<string, ComponentSchema>;
  /** The workspace's provider configurations. */
  providers?: ProviderConfig[];
  /** Milliseconds each call takes; a story shows loading with a long one. */
  delay?: number;
  /** Rejects every write with this error. */
  writeError?: Error;
}

/** An in-memory flow-definition API that behaves like the server's routes. */
export function createFlowsApi(options: FlowsApiOptions = {}) {
  const flows = (options.flows ?? sampleFlows).map((f) => structuredClone(f));
  const tools = options.tools ?? sampleTools;
  const schemas = options.schemas ?? sampleSchemas;
  const providers = options.providers ?? sampleProviders;
  let counter = 0;
  const wait = () =>
    options.delay ? new Promise<void>((r) => setTimeout(r, options.delay)) : Promise.resolve();
  const guard = () => {
    if (options.writeError) throw options.writeError;
  };
  const stored = (id: string) => {
    const found = flows.find((f) => f.id === id);
    if (!found) throw new Error(`flow ${id} not found`);
    return structuredClone(found);
  };

  const api = {
    listTools: async () => {
      await wait();
      return tools;
    },
    getToolSchema: async (name: string) => {
      await wait();
      return structuredClone(schemas[name] ?? null);
    },
    listProviderConfigs: async () => {
      await wait();
      return providers.map((p) => ({ ...p }));
    },
    listFlowDefinitions: async () => {
      await wait();
      return flows.map((f) => structuredClone(f));
    },
    getFlowDefinition: async (_ws: string, _pid: string, id: string) => {
      await wait();
      return stored(id);
    },
    createFlowDefinition: async (_ws: string, _pid: string, def: FlowDefinitionInfo) => {
      await wait();
      guard();
      const created: FlowDefinitionInfo = {
        ...structuredClone(def),
        id: def.id || `flow-${++counter}`,
        source: "project",
        created_at: "2026-09-03T10:00:00Z",
        modified_at: "2026-09-03T10:00:00Z",
      };
      flows.push(created);
      return structuredClone(created);
    },
    updateFlowDefinition: async (
      _ws: string,
      _pid: string,
      id: string,
      def: FlowDefinitionInfo,
    ) => {
      await wait();
      guard();
      const i = flows.findIndex((f) => f.id === id);
      if (i < 0) throw new Error(`flow ${id} not found`);
      flows[i] = {
        ...structuredClone(def),
        id,
        source: "project",
        created_at: flows[i].created_at,
        modified_at: "2026-09-03T10:05:00Z",
      };
      return structuredClone(flows[i]);
    },
    deleteFlowDefinition: async (_ws: string, _pid: string, id: string) => {
      await wait();
      guard();
      const i = flows.findIndex((f) => f.id === id);
      if (i < 0) throw new Error(`flow ${id} not found`);
      flows.splice(i, 1);
    },
  };
  return { api: api as unknown as ApiAdapter, flows };
}
