import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ContextFeedList } from "../components/ContextFeed";
import { ContextRevertDialog } from "../components/ContextRevertDialog";
import { ContextWidenDialog } from "../components/ContextWidenDialog";
import {
  CANDIDATE,
  CONTEXT_FEED,
  EMPTY_FEED,
  IN_FORCE,
  feedEntry,
  feedGroup,
} from "./fixtures/contextFeed";
import type { ContextFeed } from "../types/api";

const meta: Meta<typeof ContextFeedList> = {
  title: "Components/ContextFeed",
  component: ContextFeedList,
  tags: ["autodocs"],
  args: {
    feed: CONTEXT_FEED,
    keyboard: false,
    onConfirm: fn(),
    onDiscard: fn(),
    onRevert: fn(),
    onRevertSession: fn(),
    onWiden: fn(),
  },
  decorators: [
    (Story) => (
      <div className="mx-auto max-w-3xl p-6">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ContextFeedList>;

/** An agent session with a decision waiting, beside a person's own day. */
export const Default: Story = {};

/** The workspace feed, where each operation names the project it came from. */
export const AcrossProjects: Story = {
  args: {
    showProject: true,
    feed: {
      ...CONTEXT_FEED,
      groups: [
        ...CONTEXT_FEED.groups,
        feedGroup({
          id: "session:sess-2",
          session: "sess-2",
          project_key: "bowmart",
          project_name: "BowMart",
          actor: { kind: "agent", name: "claude", session: "sess-2", host: "laptop" },
          proposed: 1,
          entries: [
            feedEntry({
              ...CANDIDATE,
              id: "18",
              project_key: "bowmart",
              project_name: "BowMart",
              at: "2026-09-21T09:20:00Z",
              subject: {
                kind: "memory",
                source: "Add to basket",
                target: "Legg i handlekurv",
                target_locale: "nb-NO",
                describe: 'memory "Add to basket" into nb-NO',
              },
              evidence: [{ path: "store/checkout.json", unit: "cart.add", quote: "Add to basket" }],
            }),
          ],
        }),
      ],
    } satisfies ContextFeed,
  },
};

/** A session still recording: the summary says so rather than counting. */
export const StillWorking: Story = {
  args: {
    feed: {
      ...CONTEXT_FEED,
      groups: [
        feedGroup({
          id: "session:sess-1",
          session: "sess-1",
          proposed: 2,
          quiet: false,
          entries: [CANDIDATE, IN_FORCE],
        }),
      ],
    } satisfies ContextFeed,
  },
};

/** Nothing has been recorded in this workspace yet. */
export const Empty: Story = {
  args: { feed: EMPTY_FEED },
};

/** A project the workspace holds and no checkout here carries. */
export const NoCheckoutToDecideThrough: Story = {
  args: {
    feed: {
      ...CONTEXT_FEED,
      groups: [
        feedGroup({
          id: "session:sess-1",
          session: "sess-1",
          recipe: undefined,
          proposed: 1,
          entries: [feedEntry({ ...CANDIDATE, recipe: undefined })],
        }),
      ],
    } satisfies ContextFeed,
  },
};

/** What a rule reaches once it answers in every project. */
export const WidenToTheWorkspace: StoryObj<typeof ContextWidenDialog> = {
  render: () => (
    <ContextWidenDialog
      entry={IN_FORCE}
      to="workspace"
      onClose={fn()}
      onConfirm={fn()}
      preview={{
        to: "workspace",
        from: { level: "project", describe: "project brand=kapimart" },
        scope: { level: "workspace", describe: "workspace brand=kapimart" },
        rule: IN_FORCE.subject,
        projects: [
          { project_key: "kapimart", project_name: "KapiMart", current: true, checked_out: true },
          { project_key: "bowmart", project_name: "BowMart", current: false, checked_out: true },
          {
            project_key: "handbook",
            project_name: "Old Handbook",
            current: false,
            checked_out: false,
          },
        ],
        points: [],
        content_impact: false,
      }}
    />
  ),
};

/** What a rule reaches once it stops being specific about one axis. */
export const WidenPastAnAxis: StoryObj<typeof ContextWidenDialog> = {
  render: () => (
    <ContextWidenDialog
      entry={IN_FORCE}
      to="product"
      onClose={fn()}
      onConfirm={fn()}
      preview={{
        to: "product",
        from: { level: "project", describe: "project product=store" },
        scope: { level: "project", describe: "project" },
        rule: IN_FORCE.subject,
        projects: [],
        points: [
          {
            ref: "marketing/web",
            label: "marketing/web",
            coordinates: { product: "marketing", channel: "web" },
            collections: ["campaigns", "landing"],
          },
          {
            ref: "support/help",
            label: "support/help",
            coordinates: { product: "support", channel: "help" },
            collections: ["help-centre"],
          },
        ],
        content_impact: false,
      }}
    />
  ),
};

/** Undoing a session names how many operations go. */
export const UndoASession: StoryObj<typeof ContextRevertDialog> = {
  render: () => (
    <ContextRevertDialog
      request={{ project: "kapimart", session: "sess-1" }}
      onClose={fn()}
      onConfirm={fn()}
      scope={{
        session: "sess-1",
        operations: 4,
        rules: ['term "sign in", use "log in"'],
        subjects: [
          'term "sign in", use "log in"',
          'voice "utilise", use "use"',
          'note "prices carry no space before the currency"',
          'memory "Add to basket" into nb-NO',
        ],
      }}
    />
  ),
};
