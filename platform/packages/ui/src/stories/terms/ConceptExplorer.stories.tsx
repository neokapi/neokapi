import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ConceptExplorer } from "../../components/terms/ConceptExplorer";
import { createProvidersDecorator } from "../decorators";
import type { TermSearchResult } from "../../types/api";
import { concepts, hierarchy, graphEdges } from "./concept-fixtures";

// Mock adapter overrides to serve concept data
const mockOverrides = {
  getTerms: async (
    _ws: string,
    _q: string,
    _src: string,
    _tgt: string,
    offset: number,
    limit: number,
  ): Promise<TermSearchResult> => ({
    concepts: concepts.slice(offset, offset + limit),
    total_count: concepts.length,
  }),
  getTermCount: async () => concepts.length,
  getConceptHierarchy: async () => hierarchy,
  getGraphNeighbors: async () => [],
  getGraphEdges: async (_ws: string, nodeId: string) =>
    graphEdges.filter((e) => e.source === nodeId || e.target === nodeId),
  getGraphShortestPath: async () => ({ nodes: [], edges: [] }),
};

const emptyOverrides = {
  getTerms: async (): Promise<TermSearchResult> => ({
    concepts: [],
    total_count: 0,
  }),
  getTermCount: async () => 0,
  getConceptHierarchy: async () => [],
  getGraphNeighbors: async () => [],
  getGraphEdges: async () => [],
  getGraphShortestPath: async () => ({ nodes: [], edges: [] }),
};

const withConceptProviders = createProvidersDecorator(undefined, mockOverrides);
const withEmptyProviders = createProvidersDecorator(undefined, emptyOverrides);

const fullscreenDecorator = (Story: React.ComponentType) => (
  <div style={{ width: "100vw", height: "100vh", overflow: "hidden" }}>
    <Story />
  </div>
);

const meta: Meta<typeof ConceptExplorer> = {
  title: "Terms/ConceptExplorer",
  component: ConceptExplorer,
  tags: ["autodocs"],
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "Full-featured concept explorer with card-based list view, interactive graph view (React Flow), " +
          "and a slide-in detail panel for concept inspection and graph navigation.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof ConceptExplorer>;

/** Card-based list view — the default landing experience. Shows 9 concepts as a responsive card grid. */
export const ListView: Story = {
  decorators: [withConceptProviders, fullscreenDecorator],
  args: {
    sourceLocale: "en-US",
    targetLocales: ["de-DE", "fr-FR"],
    projects: [{ id: "proj-1", name: "Mobile App" }],
    onBack: fn(),
  },
};

/** Interactive graph view showing concept relationships via React Flow. */
export const GraphView: Story = {
  decorators: [withConceptProviders, fullscreenDecorator],
  args: {
    sourceLocale: "en-US",
    targetLocales: ["de-DE", "fr-FR"],
    projects: [{ id: "proj-1", name: "Mobile App" }],
    onBack: fn(),
  },
};

/** Empty state — no concepts in the workspace yet. Shows onboarding prompt. */
export const EmptyState: Story = {
  decorators: [withEmptyProviders, fullscreenDecorator],
  args: {
    sourceLocale: "en-US",
    targetLocales: ["de-DE", "fr-FR"],
    onBack: fn(),
  },
};

/** Single locale — only source language, no target locales configured. */
export const SingleLocale: Story = {
  decorators: [withConceptProviders, fullscreenDecorator],
  args: {
    sourceLocale: "en-US",
    targetLocales: [],
    onBack: fn(),
  },
};
