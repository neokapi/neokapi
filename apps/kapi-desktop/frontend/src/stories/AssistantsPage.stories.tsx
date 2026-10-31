import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { AssistantsPage } from "../components/AssistantsPage";
import {
  ASSISTANTS_CHECK_PENDING,
  ASSISTANTS_CLAUDE_CONNECTED,
  ASSISTANTS_HOSTS_FAILED,
  ASSISTANTS_NO_CHECKOUT,
  ASSISTANTS_NONE,
  ASSISTANTS_RULES_MISSING,
  ASSISTANTS_WITH_CHECKS,
  SESSIONS,
  SESSIONS_WITH_CHECKS,
  session,
} from "./fixtures/assistants";

/** The page at the hub's reading width, once in each theme. */
const BothThemes: Decorator = (Story) => (
  <div className="grid gap-4 p-6">
    {[false, true].map((dark) => (
      <div key={String(dark)} className={dark ? "dark" : undefined}>
        <div className="mx-auto max-w-3xl rounded-lg border bg-background p-6 text-foreground">
          <Story />
        </div>
      </div>
    ))}
  </div>
);

const meta: Meta<typeof AssistantsPage> = {
  title: "Components/AssistantsPage",
  component: AssistantsPage,
  tags: ["autodocs"],
  args: {
    tabID: "story",
    projectName: "KapiMart",
    onShowReads: () => {},
    onOpenSession: () => {},
    view: ASSISTANTS_NONE,
  },
  decorators: [BothThemes],
};

export default meta;
type Story = StoryObj<typeof AssistantsPage>;

/** No assistant connected and nothing recorded: every row offers Connect. */
export const NoneConnected: Story = {};

/** Claude Code connected with the check on, and the sessions recorded here. */
export const ClaudeCodeConnected: Story = { args: { view: ASSISTANTS_CLAUDE_CONNECTED } };

/** Claude Code connected; the check waits on the two commands that install the plugin. */
export const CheckPending: Story = { args: { view: ASSISTANTS_CHECK_PENDING } };

/** Claude Code's own files in place and the rules files deleted: not connected, Connect again. */
export const RulesFilesMissing: Story = { args: { view: ASSISTANTS_RULES_MISSING } };

/** The assistants could not be read: the error stands in their place and the sessions stay. */
export const AssistantsFailed: Story = { args: { view: ASSISTANTS_HOSTS_FAILED } };

/** A tab over the project's context alone: no assistant can be connected, the sessions stay. */
export const NoCheckout: Story = { args: { view: ASSISTANTS_NO_CHECKOUT } };

/** A long project name and a full list of sessions. */
export const ManySessions: Story = {
  args: {
    projectName: "KapiMart storefront and help centre, every market",
    view: {
      ...ASSISTANTS_CLAUDE_CONNECTED,
      sessions: [
        ...SESSIONS,
        ...Array.from({ length: 6 }, (_, i) =>
          session({
            id: `session:older-${i}`,
            actor: { kind: "agent", name: i % 2 ? "cursor" : "codex", session: `older-${i}` },
            last: `2026-10-${String(20 - i).padStart(2, "0")}T10:00:00Z`,
            operations: i + 1,
            suggestions: i,
          }),
        ),
      ],
    },
  },
};

/**
 * Sessions the check on every edit ran in: suggestions with runs that caught
 * two findings, a session that only ran the check, a clean session, and the
 * person's day with no run. The header totals what was caught.
 */
export const SessionsTheCheckRanIn: Story = { args: { view: ASSISTANTS_WITH_CHECKS } };

/** A session with suggestions whose runs caught what they reported. */
export const SessionWithNotesAndRuns: Story = {
  args: { view: { ...ASSISTANTS_WITH_CHECKS, sessions: SESSIONS_WITH_CHECKS.slice(0, 1) } },
};

/** A session that only ran the check, with a finding left failing and nothing caught. */
export const HookOnlySession: Story = {
  args: { view: { ...ASSISTANTS_WITH_CHECKS, sessions: SESSIONS_WITH_CHECKS.slice(1, 2) } },
};

/** A session whose every run was clean. */
export const CleanSession: Story = {
  args: { view: { ...ASSISTANTS_WITH_CHECKS, sessions: SESSIONS_WITH_CHECKS.slice(2, 3) } },
};
