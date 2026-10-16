import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import type { Decorator } from "@storybook/react";
import { ActivityView } from "./ActivityView";
import { sampleActivities, voiceHubOverrides } from "../../stories/voiceHubFixtures";
import { createProvidersDecorator } from "../../stories/decorators";
import type { ActivityInfo } from "../../types/api";

// The workspace's context activity as sessions beside a detail pane. The
// fixture feed is dated in June 2026, so its sessions read under their dates;
// the generated feeds below are dated from now so they read as Today.

/** A feed, filtered by type prefix the way the server filters it. */
function withFeed(activities: ActivityInfo[], pageSize = Number.POSITIVE_INFINITY): Decorator {
  return createProvidersDecorator(undefined, {
    ...voiceHubOverrides,
    listActivities: async (_ws, query) => {
      const prefixes = query?.types ?? [];
      const matching =
        prefixes.length === 0
          ? activities
          : activities.filter((a) => prefixes.some((p) => a.type.startsWith(p)));
      const start = query?.cursor ? Number(query.cursor) : 0;
      const end = Math.min(matching.length, start + Math.min(pageSize, query?.limit ?? 100));
      return {
        activities: matching.slice(start, end),
        next_cursor: end < matching.length ? String(end) : "",
      };
    },
  });
}

const minutesAgo = (m: number) => new Date(Date.now() - m * 60_000).toISOString();

const TYPES: Array<Pick<ActivityInfo, "type" | "entity_type" | "entity_id" | "summary">> = [
  {
    type: "concept.term.status_changed",
    entity_type: "concept",
    entity_id: "c-checkout",
    summary: "changed a term's status",
  },
  {
    type: "review.decided",
    entity_type: "changeset",
    entity_id: "cs-1",
    summary: "approved a change",
  },
  {
    type: "observation.added",
    entity_type: "concept",
    entity_id: "c-rival",
    summary: "recorded an observation",
  },
  {
    type: "comment.added",
    entity_type: "concept",
    entity_id: "c-basket",
    summary: "commented on a concept",
  },
  {
    type: "concept.relation.added",
    entity_type: "concept",
    entity_id: "c-checkout",
    summary: "connected two concepts",
  },
];

const PEOPLE = [
  ["alex", "Alex Romero"],
  ["sam", "Sam Okafor"],
  ["you", "You"],
] as const;

/** A busy workspace: sessions from three people over the last few days. */
const many: ActivityInfo[] = Array.from({ length: 160 }, (_, i) => {
  const [actor, name] = PEOPLE[Math.floor(i / 6) % PEOPLE.length];
  const kind = TYPES[i % TYPES.length];
  return {
    id: `act-many-${i}`,
    workspace_id: "ws-1",
    actor_id: actor,
    actor_name: name,
    created_at: minutesAgo(i * 7 + Math.floor(i / 6) * 50),
    ...kind,
  };
});

const one: ActivityInfo[] = [
  {
    id: "act-one-1",
    workspace_id: "ws-1",
    actor_id: "alex",
    actor_name: "Alex Romero",
    type: "review.decided",
    entity_type: "changeset",
    entity_id: "cs-1",
    summary: "approved a change",
    created_at: minutesAgo(3),
  },
  {
    id: "act-one-2",
    workspace_id: "ws-1",
    actor_id: "alex",
    actor_name: "Alex Romero",
    type: "concept.term.status_changed",
    entity_type: "concept",
    entity_id: "c-basket",
    summary: "changed a term's status",
    created_at: minutesAgo(9),
  },
];

/** Summaries and names longer than a row has room for. */
const long: ActivityInfo[] = [
  {
    id: "act-long-1",
    workspace_id: "ws-1",
    actor_id: "u-unknown",
    actor_name: "Maria-Luisa Fernández de la Cruz Montenegro",
    type: "pilot.started",
    entity_type: "changeset",
    entity_id: "cs-unnamed",
    stream: "release-2026-q4-holiday-campaign",
    summary:
      "started piloting a change on project-website-with-a-very-long-identifier · release-2026-q4-holiday-campaign",
    created_at: minutesAgo(12),
  },
  {
    id: "act-long-2",
    workspace_id: "ws-1",
    actor_id: "u-unknown",
    actor_name: "Maria-Luisa Fernández de la Cruz Montenegro",
    type: "comment.added",
    entity_type: "concept",
    entity_id: "c-unknown",
    summary: "commented on a concept",
    created_at: minutesAgo(15),
  },
  ...sampleActivities,
];

const meta: Meta<typeof ActivityView> = {
  title: "Context/Activity/ActivityView",
  component: ActivityView,
  tags: ["autodocs"],
  parameters: { layout: "fullscreen" },
  args: { onOpenConcept: fn(), onOpenExperiment: fn() },
  decorators: [
    (Story) => (
      <div style={{ height: 640, padding: 16, display: "flex", flexDirection: "column" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ActivityView>;

/** The fixture feed: comments, a review decided, a term moved, a pilot and an experiment. */
export const Default: Story = {
  decorators: [withFeed(sampleActivities)],
  globals: { theme: "light" },
};

export const DefaultDark: Story = {
  decorators: [withFeed(sampleActivities)],
  globals: { theme: "dark" },
};

/** The session that decided a review, selected: the decision sits in its facts. */
export const DecisionSelected: Story = {
  args: { defaultSelected: "act-2" },
  decorators: [withFeed(sampleActivities)],
  globals: { theme: "light" },
};

/** One session of two operations. */
export const OneSession: Story = {
  decorators: [withFeed(one)],
  globals: { theme: "light" },
};

/** A busy workspace read a page at a time, with Show older activity at the foot of the list. */
export const ManySessions: Story = {
  decorators: [withFeed(many, 60)],
  globals: { theme: "light" },
};

/** A long name, a long sentence and a subject nobody can name. */
export const LongContent: Story = {
  decorators: [withFeed(long)],
  globals: { theme: "light" },
};

export const Empty: Story = {
  decorators: [withFeed([])],
  globals: { theme: "light" },
};
