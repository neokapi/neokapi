import type { Meta, StoryObj } from "@storybook/react-vite";
import { ConceptsView } from "../components/ConceptsView";
import { createLocalConceptSource } from "../lib/localConceptSource";
import type { ConceptBackend, ConceptDTO, RelationDTO } from "../lib/localConceptSource";

// Three concepts + a couple of relations, in the snake_case wire shape the Wails
// backend returns. The story drives the SAME LocalConceptDataSource the desktop
// uses, just over an in-memory backend instead of the Go bindings — so opening a
// concept, relating it, removing a relation, and re-statusing a term all behave
// exactly as they do against a real local termbase.
const SAMPLE_CONCEPTS: ConceptDTO[] = [
  {
    id: "c1",
    project_id: "",
    domain: "Brand",
    definition: "Our flagship cloud product.",
    source: "brand_vocabulary",
    terms: [
      { text: "Skyline", locale: "en-US", status: "preferred" },
      {
        text: "Skyline",
        locale: "de-DE",
        status: "approved",
        validity: { tags: { market: "dach" } },
      },
      { text: "Horizon", locale: "fr-FR", status: "deprecated", note: "Legacy name." },
    ],
    created_at: new Date(Date.now() - 6 * 86400000).toISOString(),
    updated_at: new Date(Date.now() - 2 * 86400000).toISOString(),
  },
  {
    id: "c2",
    project_id: "",
    domain: "Brand",
    definition: "The compute tier of the platform.",
    source: "terminology",
    terms: [
      { text: "Skyline Compute", locale: "en-US", status: "preferred" },
      { text: "Skyline Rechner", locale: "de-DE", status: "admitted" },
    ],
    created_at: new Date(Date.now() - 5 * 86400000).toISOString(),
    updated_at: new Date(Date.now() - 86400000).toISOString(),
  },
  {
    id: "c3",
    project_id: "",
    domain: "Legal",
    definition: "A competitor offering.",
    source: "terminology",
    terms: [{ text: "Nimbus", locale: "en-US", status: "forbidden", competitor_term: true }],
    created_at: new Date(Date.now() - 4 * 86400000).toISOString(),
    updated_at: new Date(Date.now() - 3 * 86400000).toISOString(),
  },
];

const SAMPLE_RELATIONS: RelationDTO[] = [
  { id: "r1", source_id: "c2", target_id: "c1", type: "PART_OF" },
  { id: "r2", source_id: "c1", target_id: "c3", type: "COMPETITOR", note: "Watch positioning." },
];

/** A mutable in-memory backend so the editing affordances are live in the story. */
function makeMemoryBackend(): ConceptBackend {
  const concepts = structuredClone(SAMPLE_CONCEPTS);
  let relations = structuredClone(SAMPLE_RELATIONS);
  let seq = relations.length;
  const find = (id: string) => concepts.find((c) => c.id === id) ?? null;

  return {
    async searchTerms(_handle, query, _src, _tgt, offset, limit) {
      const q = query.trim().toLowerCase();
      const matched = q
        ? concepts.filter(
            (c) =>
              c.domain.toLowerCase().includes(q) ||
              c.definition.toLowerCase().includes(q) ||
              c.terms.some((t) => t.text.toLowerCase().includes(q)),
          )
        : concepts;
      return { concepts: matched.slice(offset, offset + limit), total_count: matched.length };
    },
    async getConceptForView(_handle, id) {
      const c = find(id);
      return c ? structuredClone(c) : null;
    },
    async getRelations(_handle, conceptId) {
      return relations.filter((r) => r.source_id === conceptId || r.target_id === conceptId);
    },
    async addRelation(_handle, req) {
      const rel: RelationDTO = {
        id: `r${++seq}`,
        source_id: req.source_id,
        target_id: req.target_id,
        type: req.type,
        note: req.note,
        validity:
          req.valid_from || req.valid_to || req.tags
            ? { valid_from: req.valid_from, valid_to: req.valid_to, tags: req.tags }
            : undefined,
      };
      relations.push(rel);
      return rel;
    },
    async removeRelation(_handle, relationID) {
      relations = relations.filter((r) => r.id !== relationID);
    },
    async setTermStatus(_handle, req) {
      const term = find(req.concept_id)?.terms.find(
        (t) => t.locale === req.locale && t.text === req.text,
      );
      if (term) term.status = req.status;
    },
    async updateConcept(_handle, req) {
      const c = find(req.concept_id);
      if (c) {
        c.domain = req.domain;
        c.definition = req.definition;
        c.terms = req.terms;
      }
    },
  };
}

function MemoryConceptsView() {
  const source = createLocalConceptSource("demo", makeMemoryBackend());
  return (
    <div className="p-6">
      <ConceptsView handle="demo" source={source} />
    </div>
  );
}

const meta: Meta<typeof MemoryConceptsView> = {
  title: "Pages/ConceptsView",
  component: MemoryConceptsView,
  tags: ["autodocs"],
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "The visual concept/relation workspace over a LOCAL termbase. Browse and search concepts, open one to read its story (terms, relations, tag-derived geography, constraints, synthesized timeline), then relate it to another concept or remove a relation inline — the desktop home for the editing the deleted CLI relation commands used to do.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof MemoryConceptsView>;

export const Default: Story = {};
