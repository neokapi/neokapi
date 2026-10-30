import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AssistantPanel } from "../components/AssistantPanel";
import { RulesFilesView } from "../components/RulesFilesView";
import { qk } from "../lib/queryKeys";
import { BRIEF_FILES, BRIEF_PATH, BRIEF_VIEW } from "./fixtures/agentContext";
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
    rulesFiles: RULES_FILES_WRITTEN,
    onBack: () => {},
  },
  decorators: [BothThemes, SeededBrief],
};

export default meta;
type Story = StoryObj<typeof AssistantPanel>;

/** The rules files written, reached from the Assistants page. */
export const RulesFilesWritten: Story = {};

/** Opened from Ask or Checks at a file: the brief tab, with that file chosen. */
export const OpenedOnAPath: Story = { args: { path: BRIEF_PATH } };

/** No rules file written yet: the empty state says where the rules go and offers Write them now. */
export const RulesFilesNotWritten: Story = { args: { rulesFiles: RULES_FILES_NOT_WRITTEN } };

/** The root's files written, the docs folder's not yet. */
export const RulesFilesPartlyWritten: Story = { args: { rulesFiles: RULES_FILES_PARTLY_WRITTEN } };

/** The project's context holds no rules, so kapi writes no file. */
export const NoRules: Story = { args: { rulesFiles: [] } };

/** The rules files on their own. */
export const RulesFilesOnly: StoryObj<typeof RulesFilesView> = {
  render: () => <RulesFilesView tabID={TAB} rulesFiles={RULES_FILES_PARTLY_WRITTEN} />,
};
