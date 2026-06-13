import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import type { Decorator } from "@storybook/react";
import { ActivityView } from "./ActivityView";
import { brandHubOverrides } from "../../stories/brandHubFixtures";
import { createProvidersDecorator } from "../../stories/decorators";
import type { ApiAdapter } from "../../api/adapter";
import type { ConceptStory } from "../../types/brand-graph";

// The shared brand fixtures return one canned story for every concept; the
// timeline reads best with distinct, dated events per concept, so the story
// layers per-id stories + change-set details on top.
const today = "2026-06-13";
const yesterday = "2026-06-12";
const earlier = "2026-06-09";

const stories: Record<string, ConceptStory> = {
  "c-checkout": {
    concept_id: "c-checkout",
    entries: [
      {
        kind: "comment",
        at: `${today}T11:20:00Z`,
        actor: "sam",
        summary: "Should fr-FR prefer ‘Paiement’?",
      },
      {
        kind: "observation",
        at: `${yesterday}T15:00:00Z`,
        actor: "alex",
        summary: "Rival uses ‘QuickPay checkout’ on their hero.",
      },
      {
        kind: "revision",
        at: `${earlier}T09:00:00Z`,
        actor: "alex",
        summary: "Added a German term.",
      },
    ],
  },
  "c-basket": {
    concept_id: "c-basket",
    entries: [
      {
        kind: "revision",
        at: `${today}T08:40:00Z`,
        actor: "alex",
        summary: "Promoted ‘Cart’ to preferred.",
      },
      {
        kind: "comment",
        at: `${earlier}T13:00:00Z`,
        actor: "sam",
        summary: "en-GB keeps ‘Basket’ as admitted.",
      },
    ],
  },
  "c-rival": {
    concept_id: "c-rival",
    entries: [
      {
        kind: "observation",
        at: `${yesterday}T10:10:00Z`,
        actor: "sam",
        summary: "Spotted ‘QuickPay’ in a partner email.",
      },
    ],
  },
};

const detailReviews: Record<
  string,
  { reviewer: string; verdict: "approve" | "reject"; at: string }[]
> = {
  "cs-1": [{ reviewer: "alex", verdict: "approve", at: `${today}T10:05:00Z` }],
};
const detailPilots: Record<string, { project: string; stream: string; at: string }[]> = {
  "cs-1": [{ project: "p-web", stream: "main", at: `${yesterday}T16:30:00Z` }],
};

const activityOverrides: Partial<ApiAdapter> = {
  getConceptStory: async (_ws, id) => stories[id] ?? { concept_id: id, entries: [] },
  getChangeset: async (_ws, id) => {
    const base = await brandHubOverrides.getChangeset!(_ws, id);
    return {
      ...base,
      id,
      reviews: (detailReviews[id] ?? []).map((r) => ({
        workspace_id: "ws-1",
        changeset_id: id,
        reviewer: r.reviewer,
        verdict: r.verdict,
        created_at: r.at,
      })),
      pilots: (detailPilots[id] ?? []).map((p) => ({
        workspace_id: "ws-1",
        changeset_id: id,
        project_id: p.project,
        stream: p.stream,
        created_by: "sam",
        created_at: p.at,
      })),
    };
  },
};

const populated: Decorator = createProvidersDecorator(undefined, {
  ...brandHubOverrides,
  ...activityOverrides,
});

const empty: Decorator = createProvidersDecorator(undefined, {
  ...brandHubOverrides,
  listChangesets: async () => [],
  listConcepts: async () => ({ concepts: [], total_count: 0 }),
});

const meta: Meta<typeof ActivityView> = {
  title: "Brand Hub/Activity/ActivityView",
  component: ActivityView,
  tags: ["autodocs"],
  parameters: { layout: "fullscreen" },
  args: { onOpenConcept: fn(), onOpenExperiment: fn() },
  decorators: [
    (Story) => (
      <div style={{ padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ActivityView>;

export const Default: Story = {
  decorators: [populated],
};

export const Empty: Story = {
  decorators: [empty],
};
