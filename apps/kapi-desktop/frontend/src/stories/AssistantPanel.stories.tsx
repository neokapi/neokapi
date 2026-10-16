import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AssistantPanel } from "../components/AssistantPanel";
import { AssistantHosts } from "../components/AssistantHosts";
import { RulesFilesView } from "../components/RulesFilesView";
import { qk } from "../lib/queryKeys";
import { BRIEF_FILES, BRIEF_PATH, BRIEF_VIEW } from "./fixtures/agentContext";
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

const TAB = "story";

/**
 * The brief reads through react-query, so the cache is pre-loaded: the files
 * the picker offers and the answer for one of them, which a seeded query
 * serves without reaching a backend.
 */
const SeededBrief: Decorator = (Story) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false, staleTime: Infinity } },
  });
  client.setQueryData(qk.contextOptions(TAB, "path"), BRIEF_FILES);
  client.setQueryData(qk.agentContext(TAB, BRIEF_PATH), BRIEF_VIEW);
  return (
    <QueryClientProvider client={client}>
      <Story />
    </QueryClientProvider>
  );
};

/** The panel at the hub's reading width, once in each theme. */
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

const meta: Meta<typeof AssistantPanel> = {
  title: "Components/AssistantPanel",
  component: AssistantPanel,
  tags: ["autodocs"],
  args: {
    tabID: TAB,
    hosts: HOSTS_ALL_CONNECTED,
    rulesFiles: RULES_FILES_WRITTEN,
  },
  decorators: [BothThemes, SeededBrief],
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
export const SomeConnected: Story = { args: { hosts: HOSTS_CLAUDE_DESKTOP_ABSENT } };

/** Opened from Ask or Checks at a file: the brief tab, with that file chosen. */
export const OpenedOnAPath: Story = { args: { path: BRIEF_PATH } };

/** No rules file written yet: the empty state says where the rules go and offers Write them now. */
export const RulesFilesNotWritten: Story = {
  args: { hosts: HOSTS_NONE_CONNECTED, rulesFiles: RULES_FILES_NOT_WRITTEN },
};

/** The root's files written, the docs folder's not yet. */
export const RulesFilesPartlyWritten: Story = { args: { rulesFiles: RULES_FILES_PARTLY_WRITTEN } };

/** The project's context holds no rules, so kapi writes no file. */
export const NoRules: Story = { args: { rulesFiles: [] } };

/** A tab over the context alone: connecting needs a checkout, and the tabs are left out. */
export const ContextOnly: Story = { args: { contextOnly: true } };

/** The connections on their own. */
export const HostsOnly: StoryObj<typeof AssistantHosts> = {
  render: () => <AssistantHosts tabID={TAB} hosts={HOSTS_CLAUDE_DESKTOP_ABSENT} />,
};

/** The rules files on their own. */
export const RulesFilesOnly: StoryObj<typeof RulesFilesView> = {
  render: () => <RulesFilesView tabID={TAB} rulesFiles={RULES_FILES_PARTLY_WRITTEN} />,
};
