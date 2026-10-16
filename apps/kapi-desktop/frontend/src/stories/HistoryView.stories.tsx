import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { HistoryView } from "../components/HistoryView";
import {
  AWAITING_FEED,
  CONTEXT_FEED,
  EMPTY_FEED,
  MANY_SESSIONS_FEED,
  RESET_FEED,
} from "./fixtures/contextFeed";

const meta: Meta<typeof HistoryView> = {
  title: "Components/HistoryView",
  component: HistoryView,
  tags: ["autodocs"],
  args: { onBack: fn() },
  parameters: { layout: "fullscreen" },
  decorators: [
    (Story) => (
      <div className="h-screen bg-background">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof HistoryView>;

/** Every project's sessions, newest first, the newest selected with its reset. */
export const Default: Story = { args: { feed: CONTEXT_FEED } };

/** The same in the dark theme. */
export const Dark: Story = { args: { feed: CONTEXT_FEED }, globals: { theme: "dark" } };

/** A session with two proposals waiting for a decision. */
export const ProposalAwaitingADecision: Story = {
  args: { feed: AWAITING_FEED, defaultSelected: "session:sess-1" },
};

/** The newest session is a reset, which the toolbar takes back. */
export const AfterAReset: Story = { args: { feed: RESET_FEED } };

/** A person's day selected. */
export const SessionSelected: Story = {
  args: { feed: CONTEXT_FEED, defaultSelected: "actor:person/asgeir:2026-09-21" },
};

/** A long history, scrolled in the list pane. */
export const ManySessions: Story = { args: { feed: MANY_SESSIONS_FEED } };

/** Nothing has been recorded in any project yet. */
export const Empty: Story = { args: { feed: EMPTY_FEED } };
