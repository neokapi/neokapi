import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ConceptDetailPanel } from "../../components/terms/ConceptDetailPanel";
import { createProvidersDecorator } from "../decorators";
import { concepts, neighborNodes, edgesForConcept } from "./concept-fixtures";

// The panel uses useLocales() which needs WorkspaceProvider context
const withProviders = createProvidersDecorator();

const panelDecorator = (Story: React.ComponentType) => (
  <div style={{ display: "flex", justifyContent: "flex-end", width: 420, height: "100vh" }}>
    <Story />
  </div>
);

const meta: Meta<typeof ConceptDetailPanel> = {
  title: "Terms/ConceptDetailPanel",
  component: ConceptDetailPanel,
  tags: ["autodocs"],
  decorators: [withProviders, panelDecorator],
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "Slide-in detail panel for a selected concept. Shows definition, terms grouped by locale, " +
          "graph relationships for navigation, properties, timeline, and edit/delete actions.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof ConceptDetailPanel>;

/** Authentication concept — rich multi-locale terms with graph relationships. */
export const WithRelationships: Story = {
  args: {
    concept: concepts[0], // Authentication
    edges: edgesForConcept("c-auth"),
    neighbors: neighborNodes,
    allConcepts: concepts,
    onClose: fn(),
    onEdit: fn(),
    onDelete: fn(),
    onNavigate: fn(),
  },
};

/** Translation Memory concept — two outgoing edges (BROADER, RELATED). */
export const MultipleEdges: Story = {
  args: {
    concept: concepts[4], // Translation Memory
    edges: edgesForConcept("c-tm"),
    neighbors: neighborNodes,
    allConcepts: concepts,
    onClose: fn(),
    onEdit: fn(),
    onDelete: fn(),
    onNavigate: fn(),
  },
};

/** Brand Voice concept — single outgoing RELATED edge. */
export const SingleEdge: Story = {
  args: {
    concept: concepts[8], // Brand Voice
    edges: edgesForConcept("c-brand-voice"),
    neighbors: neighborNodes,
    allConcepts: concepts,
    onClose: fn(),
    onEdit: fn(),
    onDelete: fn(),
    onNavigate: fn(),
  },
};

/** Segment concept — no outgoing edges, only incoming. */
export const NoOutgoingEdges: Story = {
  args: {
    concept: concepts[6], // Segment
    edges: edgesForConcept("c-segment"),
    neighbors: neighborNodes,
    allConcepts: concepts,
    onClose: fn(),
    onEdit: fn(),
    onDelete: fn(),
    onNavigate: fn(),
  },
};

/** Project concept — project-scoped (not workspace), with PART_OF relationship. */
export const ProjectScoped: Story = {
  args: {
    concept: concepts[3], // Project (project_id set)
    edges: edgesForConcept("c-project"),
    neighbors: neighborNodes,
    allConcepts: concepts,
    onClose: fn(),
    onEdit: fn(),
    onDelete: fn(),
    onNavigate: fn(),
  },
};

/** Concept with no relationships at all — shows the empty relationship message. */
export const NoRelationships: Story = {
  args: {
    concept: {
      id: "c-orphan",
      domain: "Testing",
      definition: "An isolated concept with no graph connections.",
      terms: [
        { text: "Orphan Concept", locale: "en-US", status: "preferred" },
        { text: "Verwaistes Konzept", locale: "de-DE", status: "proposed" },
      ],
      created_at: "2025-03-01T10:00:00Z",
      updated_at: "2025-03-01T10:00:00Z",
    },
    edges: [],
    neighbors: [],
    allConcepts: concepts,
    onClose: fn(),
    onEdit: fn(),
    onDelete: fn(),
    onNavigate: fn(),
  },
};

/** Concept with properties metadata. */
export const WithProperties: Story = {
  args: {
    concept: {
      ...concepts[0],
      properties: {
        source: "ISO 27001",
        "context-note": "Prefer 'authentication' over 'login' in documentation",
        sensitivity: "public",
      },
    },
    edges: edgesForConcept("c-auth"),
    neighbors: neighborNodes,
    allConcepts: concepts,
    onClose: fn(),
    onEdit: fn(),
    onDelete: fn(),
    onNavigate: fn(),
  },
};

/** Concept with deprecated and forbidden terms — tests visual distinction. */
export const MixedStatuses: Story = {
  args: {
    concept: {
      id: "c-mixed",
      domain: "Brand",
      definition: "A concept with terms in various lifecycle states.",
      terms: [
        { text: "Preferred Term", locale: "en-US", status: "preferred" },
        { text: "Approved Alt", locale: "en-US", status: "approved" },
        { text: "Legacy Name", locale: "en-US", status: "deprecated" },
        { text: "Never Use This", locale: "en-US", status: "forbidden" },
        { text: "Suggested Name", locale: "en-US", status: "proposed" },
        { text: "Akzeptierte Variante", locale: "de-DE", status: "admitted" },
        { text: "Bevorzugter Begriff", locale: "de-DE", status: "preferred" },
      ],
      created_at: "2025-01-10T09:00:00Z",
      updated_at: "2025-03-22T16:00:00Z",
    },
    edges: [],
    neighbors: [],
    allConcepts: concepts,
    onClose: fn(),
    onEdit: fn(),
    onDelete: fn(),
    onNavigate: fn(),
  },
};
