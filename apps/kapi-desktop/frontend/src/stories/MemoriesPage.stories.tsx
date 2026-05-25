import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { Database, Plus, FolderOpen, X, Upload, Download } from "lucide-react";
import { MemoriesPage } from "../components/MemoriesPage";
import {
  Button,
  TMBrowser,
  ResourceCard,
  type TMAdapter,
  type TMEntryDTO,
} from "@neokapi/ui-primitives";

const SAMPLE_ENTRIES: TMEntryDTO[] = [
  {
    id: "1",
    project_id: "",
    hint_src_lang: "en-US",
    variants: {
      "en-US": { locale: "en-US", text: "Hello world", runs: [{ text: "Hello world" }] },
      "fr-FR": {
        locale: "fr-FR",
        text: "Bonjour le monde",
        runs: [{ text: "Bonjour le monde" }],
      },
    },
    created_at: new Date(Date.now() - 3600000).toISOString(),
    updated_at: new Date(Date.now() - 3600000).toISOString(),
  },
  {
    id: "2",
    project_id: "proj-1",
    hint_src_lang: "en-US",
    variants: {
      "en-US": {
        locale: "en-US",
        text: "Click here to continue",
        runs: [
          { text: "Click " },
          { pcOpen: { id: "1", type: "fmt:bold", data: "<b>", equiv: "b" } },
          { text: "here" },
          { pcClose: { id: "1", type: "fmt:bold", data: "</b>", equiv: "b" } },
          { text: " to continue" },
        ],
      },
      "fr-FR": {
        locale: "fr-FR",
        text: "Cliquez ici pour continuer",
        runs: [
          { text: "Cliquez " },
          { pcOpen: { id: "1", type: "fmt:bold", data: "<b>", equiv: "b" } },
          { text: "ici" },
          { pcClose: { id: "1", type: "fmt:bold", data: "</b>", equiv: "b" } },
          { text: " pour continuer" },
        ],
      },
    },
    created_at: new Date(Date.now() - 7200000).toISOString(),
    updated_at: new Date(Date.now() - 7200000).toISOString(),
  },
  {
    id: "3",
    project_id: "",
    hint_src_lang: "en-US",
    variants: {
      "en-US": {
        locale: "en-US",
        text: "Bob is a hero",
        runs: [
          { ph: { id: "e1", type: "entity:person", data: "Bob", equiv: "Bob" } },
          { text: " is a hero" },
        ],
      },
      "fr-FR": {
        locale: "fr-FR",
        text: "Bob est un héros",
        runs: [
          { ph: { id: "e1", type: "entity:person", data: "Bob", equiv: "Bob" } },
          { text: " est un héros" },
        ],
      },
    },
    created_at: new Date(Date.now() - 86400000).toISOString(),
    updated_at: new Date(Date.now() - 86400000).toISOString(),
  },
];

function createMockAdapter(entries: TMEntryDTO[]): TMAdapter {
  let data = [...entries];

  const matchQuery = (e: TMEntryDTO, q: string) => {
    const needle = q.toLowerCase();
    return Object.values(e.variants).some((v) => v.text.toLowerCase().includes(needle));
  };

  return {
    async search(query) {
      const filtered = query ? data.filter((e) => matchQuery(e, query)) : data;
      return { entries: filtered, total_count: filtered.length };
    },
    async getEntry(id) {
      return data.find((e) => e.id === id) ?? null;
    },
    async addEntry(req) {
      const variants: TMEntryDTO["variants"] = {};
      for (const [locale, input] of Object.entries(req.variants)) {
        variants[locale] = {
          locale,
          text: input.text,
          runs: input.runs ?? [{ text: input.text }],
        };
      }
      data.push({
        id: String(Date.now()),
        project_id: req.project_id ?? "",
        hint_src_lang: req.hint_src_lang,
        variants,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      });
    },
    async updateEntry(req) {
      data = data.map((e) => {
        if (e.id !== req.entry_id) return e;
        const variants: TMEntryDTO["variants"] = {};
        for (const [locale, input] of Object.entries(req.variants)) {
          variants[locale] = {
            locale,
            text: input.text,
            runs: input.runs ?? [{ text: input.text }],
          };
        }
        return {
          ...e,
          variants,
          hint_src_lang: req.hint_src_lang || e.hint_src_lang,
          project_id: req.project_id ?? e.project_id,
          note: req.note ?? e.note,
          updated_at: new Date().toISOString(),
        };
      });
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
    async lookup(_req) {
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
        <TMBrowser adapter={adapter} />
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

/**
 * Real component with pre-loaded resources (no Wails API calls).
 */
export const WithResources: StoryObj<typeof MemoriesPage> = {
  render: () => (
    <MemoriesPage
      resources={[
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
      ]}
    />
  ),
};

/**
 * Real component with empty resources list.
 */
export const Empty: StoryObj<typeof MemoriesPage> = {
  render: () => <MemoriesPage resources={[]} />,
};

/** Loading state showing skeleton ResourceCards. */
export const Loading: StoryObj<typeof MemoriesPage> = {
  render: () => <MemoriesPage resources={[]} forceLoading />,
};
