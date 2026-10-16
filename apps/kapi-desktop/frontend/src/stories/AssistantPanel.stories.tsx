import type { Meta, StoryObj } from "@storybook/react-vite";
import { AssistantPanel } from "../components/AssistantPanel";
import { AssistantHosts } from "../components/AssistantHosts";
import { RulesFilesView } from "../components/RulesFilesView";
import {
  HOSTS_ALL_CONNECTED,
  HOSTS_CLAUDE_DESKTOP_ABSENT,
  HOSTS_NONE_CONNECTED,
} from "./fixtures/agentHosts";
import {
  RULES_FILES_NOT_WRITTEN,
  RULES_FILES_PARTLY_WRITTEN,
  RULES_FILES_WRITTEN,
} from "./fixtures/rulesFiles";

const meta: Meta<typeof AssistantPanel> = {
  title: "Components/AssistantPanel",
  component: AssistantPanel,
  tags: ["autodocs"],
  args: {
    tabID: "story",
    hosts: HOSTS_ALL_CONNECTED,
    rulesFiles: RULES_FILES_WRITTEN,
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
type Story = StoryObj<typeof AssistantPanel>;

/** Every assistant connected, and the rules files written. */
export const AllConnected: Story = {};

/** Every assistant is on this Mac and none is connected: each row offers Connect. */
export const NoneConnected: Story = { args: { hosts: HOSTS_NONE_CONNECTED } };

/**
 * Claude Code connected and Cursor not yet; VS Code, Codex and Claude Desktop
 * are not on this Mac, so they come last, muted, and Claude Desktop offers no Connect.
 */
export const ClaudeDesktopAbsent: Story = { args: { hosts: HOSTS_CLAUDE_DESKTOP_ABSENT } };

/** No rules file written yet: each is marked, and Write them now is offered. */
export const RulesFilesNotWritten: Story = {
  args: { hosts: HOSTS_NONE_CONNECTED, rulesFiles: RULES_FILES_NOT_WRITTEN },
};

/** The root's files written, the docs folder's not yet. */
export const RulesFilesPartlyWritten: Story = { args: { rulesFiles: RULES_FILES_PARTLY_WRITTEN } };

/** The project's context holds no rules, so kapi writes no file. */
export const NoRules: Story = { args: { rulesFiles: [] } };

/** A tab over the context alone: connecting needs a checkout. */
export const ContextOnly: Story = { args: { contextOnly: true } };

/** The connections on their own. */
export const HostsOnly: StoryObj<typeof AssistantHosts> = {
  render: () => <AssistantHosts tabID="story" hosts={HOSTS_CLAUDE_DESKTOP_ABSENT} />,
};

/** The rules files on their own. */
export const RulesFilesOnly: StoryObj<typeof RulesFilesView> = {
  render: () => <RulesFilesView tabID="story" rulesFiles={RULES_FILES_PARTLY_WRITTEN} />,
};
