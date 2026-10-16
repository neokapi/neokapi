import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ContextFeedList } from "../components/ContextFeed";
import { ContextResetDialog } from "../components/ContextResetDialog";
import { ContextWidenDialog, widenRuleOfEntry } from "../components/ContextWidenDialog";
import {
  AWAITING_FEED,
  CANDIDATE,
  CONTEXT_FEED,
  EMPTY_FEED,
  IN_FORCE,
  MANY_SESSIONS_FEED,
  RESET_FEED,
  feedEntry,
  feedGroup,
} from "./fixtures/contextFeed";
import type { ContextFeed } from "../types/api";

/**
 * History: one row per session in the list, newest first, and the selected
 * session's operations in the detail pane, with its reset in the toolbar. A
 * proposal keeps Keep, Edit and Drop on its row, and its evidence opens under
 * the row.
 */
const meta: Meta<typeof ContextFeedList> = {
  title: "Components/ContextFeed",
  component: ContextFeedList,
  tags: ["autodocs"],
  args: {
    feed: CONTEXT_FEED,
    keyboard: false,
    onKeep: fn(),
    onDrop: fn(),
    onResetSession: fn(),
    onResetBefore: fn(),
    onWiden: fn(),
  },
  decorators: [
    (Story) => (
      <div className="h-[560px] border border-border bg-background">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ContextFeedList>;

/** An agent session with a decision waiting, beside a person's own day. */
export const Default: Story = {};

/** The same in the dark theme. */
export const DefaultDark: Story = { globals: { theme: "dark" } };

/** Two proposals waiting in one session, beside a rule already in force. */
export const ProposalAwaitingADecision: Story = {
  args: { feed: AWAITING_FEED, defaultSelected: "session:sess-1" },
};

/** The same in the dark theme. */
export const ProposalAwaitingADecisionDark: Story = {
  args: { feed: AWAITING_FEED, defaultSelected: "session:sess-1" },
  globals: { theme: "dark" },
};

/** A person's day selected: a note they recorded, with nothing to reset. */
export const SessionSelected: Story = {
  args: { defaultSelected: "actor:person/asgeir:2026-09-21" },
};

/** A long history: the list scrolls on its own, and the oldest operations are cut off. */
export const ManySessions: Story = {
  args: { feed: MANY_SESSIONS_FEED, showProject: true },
};

/** The workspace feed, where each session names the project it came from. */
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
    defaultSelected: "session:sess-2",
  },
};

/** A session still recording: the counts say so. */
export const StillWorking: Story = {
  args: {
    feed: {
      ...CONTEXT_FEED,
      groups: [
        feedGroup({
          id: "session:sess-1",
          session: "sess-1",
          quiet: false,
          entries: [CANDIDATE, IN_FORCE],
        }),
      ],
    } satisfies ContextFeed,
  },
};

/** A person reset the project to before claude's session; the toolbar takes the reset back. */
export const AfterAReset: Story = { args: { feed: RESET_FEED } };

/** The same in the dark theme. */
export const AfterAResetDark: Story = {
  args: { feed: RESET_FEED },
  globals: { theme: "dark" },
};

/** The session the reset set aside, still listed, with its operations marked. */
export const SessionSetAside: Story = {
  args: {
    feed: {
      ...RESET_FEED,
      groups: RESET_FEED.groups.map((g) =>
        g.id === "session:sess-1"
          ? {
              ...g,
              awaiting: 0,
              entries: g.entries.map((e) => ({
                ...e,
                status: "reset",
                decidable: false,
                droppable: false,
              })),
            }
          : g,
      ),
    } satisfies ContextFeed,
    defaultSelected: "session:sess-1",
  },
};

/** Nothing has been recorded in this workspace yet. */
export const Empty: Story = {
  args: { feed: EMPTY_FEED },
};

/** A workspace open for reading only: the reset cannot be taken back here. */
export const ReadOnly: Story = {
  args: { feed: { ...RESET_FEED, read_only: true } },
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
      rule={widenRuleOfEntry(IN_FORCE)}
      to="workspace"
      onClose={fn()}
      onConfirm={fn()}
      preview={{
        to: "workspace",
        from: { level: "project", describe: "project brand=kapimart" },
        scope: { level: "workspace", describe: "workspace brand=kapimart" },
        rule: IN_FORCE.subject,
        projects: [
          { project_key: "bowmart", project_name: "BowMart", checked_out: true },
          { project_key: "handbook", project_name: "Old Handbook", checked_out: false },
        ],
        points: [],
        units: [
          {
            project_key: "bowmart",
            document: "docs/checkout.md",
            unit: "p4",
            text: "Utilise the saved basket to finish later.",
            matches: 1,
          },
        ],
        coverage: {
          examined: [{ project_key: "bowmart", project_name: "BowMart", units: 63, matched: 1 }],
          not_examined: [
            {
              project_key: "kapimart",
              project_name: "KapiMart",
              reason: "no projection of its content is built on this machine",
            },
            {
              project_key: "handbook",
              project_name: "Old Handbook",
              reason: "no checkout on this machine",
            },
          ],
          truncated: false,
        },
      }}
    />
  ),
};

/** What a rule reaches once it stops being specific about one axis. */
export const WidenPastAnAxis: StoryObj<typeof ContextWidenDialog> = {
  render: () => (
    <ContextWidenDialog
      rule={widenRuleOfEntry(IN_FORCE)}
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
            project_key: "kapimart",
            ref: "marketing/web",
            label: "marketing/web",
            coordinates: { product: "marketing", channel: "web" },
            collections: ["campaigns", "landing"],
          },
          {
            project_key: "kapimart",
            ref: "support/help",
            label: "support/help",
            coordinates: { product: "support", channel: "help" },
            collections: ["help-centre"],
          },
        ],
        units: [
          {
            project_key: "kapimart",
            document: "campaigns/spring.md",
            unit: "p7",
            text: "Utilise the spring offer before it ends.",
            matches: 1,
          },
          {
            project_key: "kapimart",
            document: "help/returns.md",
            unit: "p2",
            text: "Utilise the returns portal to print a label.",
            matches: 1,
          },
        ],
        coverage: {
          examined: [{ project_key: "kapimart", project_name: "KapiMart", units: 140, matched: 2 }],
          not_examined: [],
          truncated: false,
        },
      }}
    />
  ),
};

/** Resetting to before a session names what it sets aside. */
export const ResetToBeforeASession: StoryObj<typeof ContextResetDialog> = {
  render: () => (
    <ContextResetDialog
      request={{ project: "kapimart", before: "sess-1" }}
      onClose={fn()}
      onConfirm={fn()}
      scope={{
        before: "sess-1",
        set_aside: 4,
        decisions: 1,
        restored: 0,
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
