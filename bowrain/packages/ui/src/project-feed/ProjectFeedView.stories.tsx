import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Decorator } from "@storybook/react";
import { fn } from "storybook/test";
import type { FeedEntry } from "@neokapi/bowrain-client";
import { ProjectFeedView } from "./ProjectFeedView";
import { feedEveryKind, feedLong, feedMany } from "./fixtures";
import { createProvidersDecorator } from "../stories/decorators";

// A project's feed as list and detail: one row per entry under its day, the
// selected entry's counts and facts beside it. The fixtures are dated from
// now, so they read under Today and Yesterday.

/** A feed served page by page and filtered the way the server filters it. */
function withFeed(entries: FeedEntry[], pageSize = 50): Decorator {
  return createProvidersDecorator(undefined, {
    listProjectFeed: async (_ws, _project, query) => {
      const matching = entries.filter(
        (e) =>
          (!query?.kinds?.length || query.kinds.includes(e.kind as never)) &&
          (!query?.actorKind || e.actor.kind === query.actorKind) &&
          (!query?.locale || (e.locales ?? []).includes(query.locale)),
      );
      const start = query?.cursor ? Number(query.cursor) : 0;
      const end = Math.min(matching.length, start + Math.min(pageSize, query?.limit ?? 50));
      return {
        entries: matching.slice(start, end),
        next_cursor: end < matching.length ? String(end) : undefined,
      };
    },
  });
}

const meta: Meta<typeof ProjectFeedView> = {
  title: "Pages/Project/ProjectFeedView",
  component: ProjectFeedView,
  tags: ["autodocs"],
  parameters: { layout: "fullscreen" },
  args: {
    workspace: "acme",
    projectId: "p-docs",
    languages: ["nb", "de", "fr"],
    onOpenLink: fn(),
  },
  decorators: [
    (Story) => (
      <div style={{ height: 680, padding: 16, display: "flex", flexDirection: "column" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ProjectFeedView>;

/** One entry of each kind, the newest push selected. */
export const EveryKind: Story = {
  decorators: [withFeed(feedEveryKind)],
  globals: { theme: "light" },
};

export const EveryKindDark: Story = {
  decorators: [withFeed(feedEveryKind)],
  globals: { theme: "dark" },
};

/** An agent's context change, selected: the session and the member it ran for. */
export const AgentContextChange: Story = {
  args: { defaultSelected: "e-context" },
  decorators: [withFeed(feedEveryKind)],
  globals: { theme: "light" },
};

/** A recipe change, selected: the values before and after. */
export const RecipeChange: Story = {
  args: { defaultSelected: "e-recipe" },
  decorators: [withFeed(feedEveryKind)],
  globals: { theme: "light" },
};

export const RecipeChangeDark: Story = {
  args: { defaultSelected: "e-recipe" },
  decorators: [withFeed(feedEveryKind)],
  globals: { theme: "dark" },
};

/** One entry. */
export const OneEntry: Story = {
  decorators: [withFeed(feedEveryKind.slice(0, 1))],
  globals: { theme: "light" },
};

/** A busy project read a page at a time, with Show older activity at the foot. */
export const ManyEntries: Story = {
  decorators: [withFeed(feedMany, 40)],
  globals: { theme: "light" },
};

/** Long names, a long stream and a recipe change with every axis. */
export const LongContent: Story = {
  args: { defaultSelected: "e-long-recipe" },
  decorators: [withFeed(feedLong)],
  globals: { theme: "light" },
};

export const Empty: Story = {
  decorators: [withFeed([])],
  globals: { theme: "light" },
};

export const EmptyDark: Story = {
  decorators: [withFeed([])],
  globals: { theme: "dark" },
};
