import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Decorator } from "@storybook/react";
import { fn } from "storybook/test";
import type { FeedEntry } from "@neokapi/bowrain-client";
import { ProjectFeedView } from "./ProjectFeedView";
import { feedEveryKind, feedFolded, feedLong, feedMany } from "./fixtures";
import { createProvidersDecorator } from "../stories/decorators";

// A project's feed as a column of cards: who did what, when, and a preview of
// the change. The fixtures are dated from now, so they read under Today and
// Yesterday.

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
    onOpenLanguage: fn(),
  },
  decorators: [
    (Story) => (
      <div style={{ padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ProjectFeedView>;

/** One card of each kind: push, run, agent context change, review, milestone, recipe and change set. */
export const EveryKind: Story = {
  decorators: [withFeed(feedEveryKind)],
  globals: { theme: "light" },
};

export const EveryKindDark: Story = {
  decorators: [withFeed(feedEveryKind)],
  globals: { theme: "dark" },
};

/** Three runs close together fold into one card; the quiet run is left out. */
export const FoldedRuns: Story = {
  decorators: [withFeed(feedFolded)],
  globals: { theme: "light" },
};

/** One card. */
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
  decorators: [withFeed(feedLong)],
  globals: { theme: "light" },
};

export const LongContentDark: Story = {
  decorators: [withFeed(feedLong)],
  globals: { theme: "dark" },
};

export const Empty: Story = {
  decorators: [withFeed([])],
  globals: { theme: "light" },
};
