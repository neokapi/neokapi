import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { BookOpen, Plus, FolderOpen, X, Upload } from "lucide-react";
import {
  Button,
  TermbaseBrowser,
  ResourceCard,
  type TermbaseAdapter,
  type ConceptDTO,
} from "@neokapi/ui-primitives";

const SAMPLE_CONCEPTS: ConceptDTO[] = [
  {
    id: "c1",
    project_id: "",
    domain: "Legal",
    definition: "A legally binding agreement between parties",
    source: "terminology",
    terms: [
      { text: "contract", locale: "en-US", status: "preferred" },
      { text: "contrat", locale: "fr-FR", status: "approved" },
      { text: "Vertrag", locale: "de-DE", status: "approved" },
    ],
    created_at: new Date(Date.now() - 3600000).toISOString(),
    updated_at: new Date(Date.now() - 3600000).toISOString(),
  },
  {
    id: "c2",
    project_id: "proj-1",
    domain: "Software",
    definition: "A reusable interface component",
    source: "terminology",
    terms: [
      { text: "widget", locale: "en-US", status: "preferred" },
      { text: "widget", locale: "fr-FR", status: "approved" },
      { text: "gadget", locale: "fr-FR", status: "deprecated", note: "Use 'widget' instead" },
    ],
    created_at: new Date(Date.now() - 86400000).toISOString(),
    updated_at: new Date(Date.now() - 86400000).toISOString(),
  },
  {
    id: "c3",
    project_id: "",
    domain: "Medical",
    definition: "Inflammation of the appendix",
    source: "terminology",
    terms: [
      { text: "appendicitis", locale: "en-US", status: "preferred", part_of_speech: "noun" },
      { text: "appendicite", locale: "fr-FR", status: "approved", gender: "feminine" },
    ],
    created_at: new Date(Date.now() - 172800000).toISOString(),
    updated_at: new Date(Date.now() - 172800000).toISOString(),
  },
];

function createMockAdapter(concepts: ConceptDTO[]): TermbaseAdapter {
  let data = [...concepts];
  return {
    async search(query) {
      const filtered = query
        ? data.filter(
            (c) =>
              c.terms.some((t) => t.text.toLowerCase().includes(query.toLowerCase())) ||
              c.domain.toLowerCase().includes(query.toLowerCase()),
          )
        : data;
      return { concepts: filtered, total_count: filtered.length };
    },
    async getConcept(id) {
      return data.find((c) => c.id === id) ?? null;
    },
    async addConcept(req) {
      data.push({
        id: String(Date.now()),
        project_id: req.project_id ?? "",
        domain: req.domain,
        definition: req.definition,
        source: "terminology",
        terms: req.terms,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      });
    },
    async updateConcept(req) {
      data = data.map((c) =>
        c.id === req.concept_id
          ? {
              ...c,
              domain: req.domain,
              definition: req.definition,
              terms: req.terms,
              updated_at: new Date().toISOString(),
            }
          : c,
      );
    },
    async deleteConcept(id) {
      data = data.filter((c) => c.id !== id);
    },
    async deleteConcepts(ids) {
      const idSet = new Set(ids);
      data = data.filter((c) => !idSet.has(c.id));
    },
  };
}

function SimulatedTermbasesPage() {
  const [handle, setHandle] = useState<string | null>(null);
  const adapter = handle ? createMockAdapter(SAMPLE_CONCEPTS) : null;

  const resources = [
    {
      name: "my-glossary",
      path: "~/.config/kapi/termbases/my-glossary.db",
      size: 262144,
      modified: new Date(Date.now() - 7200000).toISOString(),
    },
    {
      name: "brand-terms",
      path: "~/.config/kapi/termbases/brand-terms.db",
      size: 131072,
      modified: new Date(Date.now() - 172800000).toISOString(),
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
            <h1 className="text-lg font-semibold">my-glossary</h1>
          </div>
          <div className="flex gap-2">
            <Button variant="outline" size="sm">
              <Upload size={12} /> Import CSV
            </Button>
          </div>
        </div>
        <TermbaseBrowser adapter={adapter} />
      </div>
    );
  }

  return (
    <div className="p-6">
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-xl font-semibold">Termbases</h1>
        <div className="flex gap-2">
          <Button variant="outline" size="sm">
            <FolderOpen size={12} /> Open File...
          </Button>
          <Button size="sm">
            <Plus size={12} /> New Termbase
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
            icon={<BookOpen size={18} />}
            onClick={() => setHandle("mock-handle")}
          />
        ))}
      </div>
    </div>
  );
}

const meta: Meta<typeof SimulatedTermbasesPage> = {
  title: "Pages/TermbasesPage",
  component: SimulatedTermbasesPage,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "Termbase browser with resource picker, card-based concept display, multi-locale terms with status badges, and CRUD operations.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof SimulatedTermbasesPage>;

export const Default: Story = {};
