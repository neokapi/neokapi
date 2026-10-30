import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ContextHub } from "../components/ContextHub";
import { qk } from "../lib/queryKeys";
import { CONTEXT_DIGEST, EMPTY_DIGEST, LAST_LOOKED } from "./fixtures/contextDigest";
import { RESET_FEED } from "./fixtures/contextFeed";
import { ASSISTANTS_CLAUDE_CONNECTED } from "./fixtures/assistants";
import { RULES_FILES_WRITTEN } from "./fixtures/rulesFiles";
import { LOCAL_VIEW } from "./fixtures/sharing";
import { DECIDE_VIEW, EMPTY_DECIDE_VIEW } from "./fixtures/rulesDecide";
import type { ContextDigest, RulesDecideView } from "../types/api";

const TAB = "tab-fernwell";

/**
 * The hub reads every section through react-query, so a story pre-loads the
 * cache: a seeded query answers from it and never reaches the backend.
 */
function seeded(digest: ContextDigest, decide: RulesDecideView = DECIDE_VIEW) {
  return function SeededHub(Story: React.ComponentType) {
    const client = new QueryClient({
      defaultOptions: {
        queries: { retry: false, refetchOnWindowFocus: false, staleTime: Infinity },
      },
    });
    // The digest's first read, then the read from the instant it answered.
    client.setQueryData(qk.projectContextDigest(TAB, ""), digest);
    client.setQueryData(qk.projectContextDigest(TAB, digest.since ?? ""), digest);
    client.setQueryData(qk.rulesDecide(TAB), decide);
    client.setQueryData(qk.contextFeed(digest.project), RESET_FEED);
    client.setQueryData(qk.shareView(TAB), LOCAL_VIEW);
    client.setQueryData(qk.assistants(TAB), ASSISTANTS_CLAUDE_CONNECTED);
    client.setQueryData(qk.rulesFiles(TAB), RULES_FILES_WRITTEN);
    return (
      <QueryClientProvider client={client}>
        <div className="h-[56rem]">
          <Story />
        </div>
      </QueryClientProvider>
    );
  };
}

const meta: Meta<typeof ContextHub> = {
  title: "Components/ContextHub",
  component: ContextHub,
  tags: ["autodocs"],
  args: { tabID: TAB, projectName: "Fernwell", hasTargetLanguages: true },
  decorators: [seeded(CONTEXT_DIGEST)],
  parameters: { layout: "fullscreen" },
};

export default meta;
type Story = StoryObj<typeof ContextHub>;

/** The hub opens on Decide, one question per suggestion. */
export const WhatsNew: Story = {};

/** Nothing waits for a decision: the rules count and the way to the Map. */
export const NothingRecorded: Story = {
  decorators: [
    seeded(
      { ...EMPTY_DIGEST, project: CONTEXT_DIGEST.project, since: LAST_LOOKED },
      EMPTY_DECIDE_VIEW,
    ),
  ],
};

/** A project with no checkout here: Decide reads the digest, without decisions. */
export const ContextOnlyDecide: Story = { args: { contextOnly: true } };

/** Assistants: each assistant and its connection, then the sessions recorded here. */
export const Assistants: Story = { args: { section: "agent" } };

/** Sessions, reached from Assistants: the project's own, with a reset that can be taken back. */
export const Sessions: Story = { args: { section: "history" } };

/** How far the project's context reaches, on this Mac for now. */
export const Share: Story = { args: { section: "share" } };

/** A project with no checkout here: the sections that read files are left out. */
export const ContextOnly: Story = { args: { contextOnly: true, section: "share" } };
