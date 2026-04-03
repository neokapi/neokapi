import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState, useCallback } from "react";
import { Database, Plus, FolderOpen, X, Upload, Download } from "lucide-react";
import {
  Button,
  TMBrowser,
  ResourceCard,
  type TMAdapter,
  type TMSearchResult,
  type TMEntryDTO,
  type TMMatchDTO,
  type AnnotateResult,
} from "@neokapi/ui-primitives";

const SAMPLE_ENTRIES: TMEntryDTO[] = [
  {
    id: "1",
    source_text: "Hello world",
    target_text: "Bonjour le monde",
    source_coded: "Hello world",
    target_coded: "Bonjour le monde",
    source_spans: [],
    target_spans: [],
    source_locale: "en-US",
    target_locale: "fr-FR",
    project_id: "",
    created_at: new Date(Date.now() - 3600000).toISOString(),
    updated_at: new Date(Date.now() - 3600000).toISOString(),
  },
  {
    id: "2",
    source_text: "Click here to continue",
    target_text: "Cliquez ici pour continuer",
    source_coded: "Click \uE001here\uE002 to continue",
    target_coded: "Cliquez \uE001ici\uE002 pour continuer",
    source_spans: [
      { span_type: "opening", type: "fmt:bold", id: "1", data: "<b>" },
      { span_type: "closing", type: "fmt:bold", id: "1", data: "</b>" },
    ],
    target_spans: [
      { span_type: "opening", type: "fmt:bold", id: "1", data: "<b>" },
      { span_type: "closing", type: "fmt:bold", id: "1", data: "</b>" },
    ],
    source_locale: "en-US",
    target_locale: "fr-FR",
    project_id: "proj-1",
    created_at: new Date(Date.now() - 7200000).toISOString(),
    updated_at: new Date(Date.now() - 7200000).toISOString(),
  },
  {
    id: "3",
    source_text: " is a hero",
    target_text: " est un héros",
    source_coded: "\uE003 is a hero",
    target_coded: "\uE003 est un héros",
    source_spans: [{ span_type: "placeholder", type: "entity:person", id: "e1", data: "Bob" }],
    target_spans: [{ span_type: "placeholder", type: "entity:person", id: "e1", data: "Bob" }],
    source_locale: "en-US",
    target_locale: "fr-FR",
    project_id: "",
    created_at: new Date(Date.now() - 86400000).toISOString(),
    updated_at: new Date(Date.now() - 86400000).toISOString(),
  },
];

function createMockAdapter(entries: TMEntryDTO[]): TMAdapter {
  let data = [...entries];
  return {
    async search(query) {
      const filtered = query
        ? data.filter(
            (e) =>
              e.source_text.toLowerCase().includes(query.toLowerCase()) ||
              e.target_text.toLowerCase().includes(query.toLowerCase()),
          )
        : data;
      return { entries: filtered, total_count: filtered.length };
    },
    async getEntry(id) {
      return data.find((e) => e.id === id) ?? null;
    },
    async addEntry(req) {
      data.push({
        id: String(Date.now()),
        source_text: req.source,
        target_text: req.target,
        source_coded: req.source,
        target_coded: req.target,
        source_spans: [],
        target_spans: [],
        source_locale: req.source_locale,
        target_locale: req.target_locale,
        project_id: req.project_id ?? "",
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      });
    },
    async updateEntry(req) {
      data = data.map((e) =>
        e.id === req.entry_id
          ? {
              ...e,
              target_text: req.target,
              target_coded: req.target,
              updated_at: new Date().toISOString(),
            }
          : e,
      );
    },
    async deleteEntry(id) {
      data = data.filter((e) => e.id !== id);
    },
    async deleteEntries(ids) {
      const idSet = new Set(ids);
      data = data.filter((e) => !idSet.has(e.id));
    },
    async annotateEntities(req) {
      return {
        entries_updated: req.entry_ids.length,
        entities_added: req.entry_ids.length * req.patterns.length,
      };
    },
    async lookup(req) {
      return SAMPLE_ENTRIES.slice(0, 2).map((e, i) => ({
        entry: e,
        score: i === 0 ? 1.0 : 0.85,
        match_type: i === 0 ? "generalized-exact" : "fuzzy",
        entity_adaptations:
          i === 0
            ? [
                {
                  placeholder_id: "e1",
                  type: "entity:person",
                  stored_value: "John",
                  current_value: "Bob",
                },
              ]
            : [],
      }));
    },
  };
}

function SimulatedMemoriesPage() {
  const [handle, setHandle] = useState<string | null>(null);
  const adapter = handle ? createMockAdapter(SAMPLE_ENTRIES) : null;

  const resources = [
    {
      name: "my-project",
      path: "~/.config/kapi/tm/my-project.db",
      size: 524288,
      modified: new Date(Date.now() - 3600000).toISOString(),
    },
    {
      name: "global-tm",
      path: "~/.config/kapi/tm/global-tm.db",
      size: 1048576,
      modified: new Date(Date.now() - 86400000).toISOString(),
    },
  ];

  if (handle && adapter) {
    return (
      <div className="p-6">
        <div className="flex items-center justify-between mb-6">
          <div className="flex items-center gap-3">
            <button
              onClick={() => setHandle(null)}
              className="p-1 rounded hover:bg-accent text-muted-foreground hover:text-foreground"
            >
              <X size={16} />
            </button>
            <h1 className="text-lg font-semibold">my-project</h1>
          </div>
          <div className="flex gap-2">
            <Button variant="outline" size="sm">
              <Upload size={12} /> Import TMX
            </Button>
            <Button variant="outline" size="sm">
              <Download size={12} /> Export TMX
            </Button>
          </div>
        </div>
        <TMBrowser adapter={adapter} showLookup />
      </div>
    );
  }

  return (
    <div className="p-6">
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-xl font-semibold">Translation Memories</h1>
        <div className="flex gap-2">
          <Button variant="outline" size="sm">
            <FolderOpen size={12} /> Open File...
          </Button>
          <Button size="sm">
            <Plus size={12} /> Create TM
          </Button>
        </div>
      </div>
      <div className="grid grid-cols-1 md:grid-cols-2 gap-2">
        {resources.map((r) => (
          <ResourceCard
            key={r.path}
            name={r.name}
            path={r.path}
            size={r.size}
            modified={r.modified}
            icon={<Database size={18} />}
            onClick={() => setHandle("mock-handle")}
          />
        ))}
      </div>
    </div>
  );
}

const meta: Meta<typeof SimulatedMemoriesPage> = {
  title: "Pages/MemoriesPage",
  component: SimulatedMemoriesPage,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "Translation Memory browser with resource picker, full-text search, inline code rendering, entity-aware lookup, and batch entity annotation.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof SimulatedMemoriesPage>;

export const Default: Story = {};
