// Shared mock data for the kapi-lab Storybook stories. These mirror the shapes
// the WASM engine returns (ContentTree / ContentNode), so the presentational
// components can be exercised without booting the runtime.
import type { ContentNode, ContentTree } from "@neokapi/ui-primitives/preview";
import type { LabRuntime } from "../useLabRuntime";

export const richBlock: ContentNode = {
  kind: "block",
  id: "greeting",
  type: "json:value",
  translatable: true,
  sourceLocale: "en-US",
  source: [
    { text: "Hello, " },
    { ph: { id: "1", type: "var", data: "{name}", equiv: "{name}" } },
    { text: "! Read the " },
    { pcOpen: { id: "2", type: "tag", data: "<a>", equiv: "<a>" } },
    { text: "docs" },
    { pcClose: { id: "2", data: "</a>", equiv: "</a>" } },
    { text: "." },
  ],
  targets: {
    "fr-FR": [
      { text: "Bonjour, " },
      { ph: { id: "1", data: "{name}", equiv: "{name}" } },
      { text: " ! Consultez la " },
      { pcOpen: { id: "2", data: "<a>", equiv: "<a>" } },
      { text: "documentation" },
      { pcClose: { id: "2", data: "</a>", equiv: "</a>" } },
      { text: "." },
    ],
  },
  targetMeta: {
    "fr-FR": {
      status: "reviewed",
      score: 0.98,
      origin: { kind: "ai", engine: "anthropic", tool: "ai-translate" },
    },
  },
  segments: [
    { id: "s1", start: 0, end: 3 },
    { id: "s2", start: 3, end: 7 },
  ],
  overlays: [
    {
      type: "segmentation",
      side: "source",
      spans: [
        {
          id: "s1",
          range: { startRun: 0, startOffset: 0, endRun: 3, endOffset: 0 },
          text: "Hello, {name}! ",
        },
        {
          id: "s2",
          range: { startRun: 3, startOffset: 0, endRun: 7, endOffset: 0 },
          text: "Read the docs.",
        },
      ],
    },
    {
      type: "term",
      side: "source",
      spans: [
        {
          id: "t1",
          range: { startRun: 4, startOffset: 0, endRun: 5, endOffset: 0 },
          text: "docs",
          props: { termbase: "glossary", match: "exact" },
        },
      ],
    },
    {
      type: "qa",
      side: "fr-FR",
      spans: [
        {
          id: "q1",
          range: { startRun: 2, startOffset: 0, endRun: 3, endOffset: 0 },
          text: " ! ",
          props: { rule: "spacing", severity: "info" },
        },
      ],
    },
  ],
  annotations: [
    {
      key: "note-1",
      type: "note",
      summary: "Keep the link to the docs.",
      fields: { from: "developer", priority: 1, annotates: "source" },
    },
    {
      key: "alt-1",
      type: "alt-translation",
      summary: "Salut, {name} !",
      fields: { locale: "fr-FR", matchType: "fuzzy", score: 0.82, engine: "tm" },
    },
  ],
  properties: { path: "$.greeting", source: "messages.json" },
  identity: "a1b2c3d4e5f60718",
};

export const plainBlock: ContentNode = {
  kind: "block",
  id: "farewell",
  type: "json:value",
  translatable: true,
  source: [{ text: "See you tomorrow" }],
};

export const mockTree: ContentTree = {
  format: "json",
  stats: { layers: 1, groups: 1, blocks: 2, data: 0, media: 0, runs: 8 },
  root: [
    {
      kind: "layer",
      id: "document",
      name: "messages.json",
      format: "json",
      locale: "en-US",
      children: [
        richBlock,
        {
          kind: "group",
          id: "cart",
          name: "cart",
          children: [plainBlock],
        },
      ],
    },
  ],
};

export const sampleJson = `{
  "greeting": "Hello, {name}!",
  "cart": {
    "empty": "Your cart is empty",
    "checkout": "Proceed to checkout"
  },
  "farewell": "See you tomorrow"
}
`;

// A LabRuntime stand-in for OutputView stories: returns the given bytes + tree
// without booting WASM. Only the methods OutputView calls are real.
export function makeMockRuntime(text: string, tree: ContentTree): LabRuntime {
  const bytes = new TextEncoder().encode(text);
  const rt: Partial<LabRuntime> = {
    status: "ready",
    error: null,
    ready: true,
    readBytes: () => bytes,
    readFile: () => text,
    inspect: async () => ({ ok: true, format: tree.format, tree }),
    mkdir: () => {},
    writeFile: (n: string) => `/project/${n}`,
    trace: async () => ({ ok: false }),
    run: async () => 0,
    runCapture: async () => ({ code: 0, output: "" }),
    klf: () => ({ ok: false }),
  };
  return rt as LabRuntime;
}
