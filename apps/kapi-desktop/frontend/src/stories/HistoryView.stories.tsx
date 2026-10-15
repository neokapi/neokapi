import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { HistoryView } from "../components/HistoryView";
import { CONTEXT_FEED } from "./fixtures/contextFeed";

const meta: Meta<typeof HistoryView> = {
  title: "Components/HistoryView",
  component: HistoryView,
  tags: ["autodocs"],
  args: { onBack: fn() },
  parameters: { layout: "fullscreen" },
};

export default meta;
type Story = StoryObj<typeof HistoryView>;

/** Every project's sessions, newest first, each with the reset it offers. */
export const Default: Story = { args: { feed: CONTEXT_FEED } };

/** Nothing has been recorded in any project yet. */
export const Empty: Story = {
  args: { feed: { groups: [], truncated: false, read_only: false } },
};
