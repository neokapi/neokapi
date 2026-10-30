import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MapPage, MapView } from "../components/MapPage";
import { ConnectAICard } from "../components/ConnectAICard";
import {
  MAP_ALL_KEPT,
  MAP_EMPTY,
  MAP_PARTLY_READ,
  MAP_PENDING,
  MAP_SOURCE_ONLY_UNCHECKED,
  MAP_SUGGESTED_ONLY,
} from "./fixtures/mapSummary";

const TAB = "tab-fernwell";

/** The page reads through react-query; a story hands it the summary instead. */
function withClient(Story: React.ComponentType) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false, staleTime: Infinity } },
  });
  return (
    <QueryClientProvider client={client}>
      <div className="h-[56rem]">
        <Story />
      </div>
    </QueryClientProvider>
  );
}

/**
 * The Map, the page a project tab opens on: the files, what applies to them,
 * and four counts. A suggestion's Decide opens the Decide view on that row; a
 * file's Open shows the file.
 */
const meta: Meta<typeof MapPage> = {
  title: "Pages/MapPage",
  component: MapPage,
  tags: ["autodocs"],
  args: {
    tabID: TAB,
    projectName: "Fernwell",
    summary: MAP_PENDING,
    deeperLookModel: { connected: true, provider: "demo", model: "demo-stub" },
    onDecide: fn(),
    onNavigate: fn(),
  },
  decorators: [withClient],
  parameters: { layout: "fullscreen" },
};

export default meta;
type Story = StoryObj<typeof MapPage>;

/** An assistant noticed names, words and a way of writing; one disagreement waits. */
export const SuggestionsPending: Story = {};

/** Suggestions and no disagreement: the Decide button carries the count alone. */
export const SuggestionsOnly: Story = {
  args: { summary: MAP_SUGGESTED_ONLY },
};

/** kapi read fewer files than the project has, so the line under the title names how many. */
export const PartlyRead: Story = {
  args: { summary: MAP_PARTLY_READ },
};

/** A new project whose collections match no file yet. */
export const Empty: Story = {
  args: { projectName: "Untitled", summary: MAP_EMPTY },
};

/** Every suggestion kept: the rules hold and no file has a finding. */
export const AllKept: Story = {
  args: { summary: MAP_ALL_KEPT },
};

/** A source-only project not checked yet in this session. */
export const NotCheckedYet: Story = {
  args: { summary: MAP_SOURCE_ONLY_UNCHECKED },
};

/** A checks run over the whole project in progress. */
export const Checking: StoryObj<typeof MapView> = {
  render: () => (
    <MapView
      projectName="Fernwell"
      summary={MAP_PENDING}
      running
      onRun={fn()}
      onDecide={fn()}
      onNavigate={fn()}
      onOpenFile={fn()}
    />
  ),
};

/** The Map as drawn, with the deeper look's row in a given state. */
function DeeperLookMap(props: Partial<React.ComponentProps<typeof MapView>>) {
  return (
    <MapView
      projectName="Fernwell"
      summary={MAP_PENDING}
      onRun={fn()}
      onDecide={fn()}
      onNavigate={fn()}
      onOpenFile={fn()}
      onDeeperLook={fn()}
      deeperLook={{ model: "demo-stub" }}
      {...props}
    />
  );
}

/** No model connected: the deeper look's button opened the Connect your AI card. */
export const DeeperLookNoModel: StoryObj<typeof MapView> = {
  render: () => (
    <DeeperLookMap
      deeperLook={{ model: null }}
      connectAI={
        <ConnectAICard
          detection={{ detected: [], configured: false }}
          description="Pick the model kapi reads your files with. You can change it any time in Settings."
        />
      }
    />
  ),
};

/** The connected model reads the files: the quiet progress state, with its name. */
export const DeeperLookRunning: StoryObj<typeof MapView> = {
  render: () => <DeeperLookMap deeperLook={{ model: "demo-stub", running: true }} />,
};

/** The run filed three suggestions, which appear as rows; the line says so for a few seconds. */
export const DeeperLookDone: StoryObj<typeof MapView> = {
  render: () => (
    <DeeperLookMap
      deeperLook={{
        model: "demo-stub",
        result: {
          provider: "demo",
          model: "demo-stub",
          filed: 3,
          asked: 251,
          blocks: 251,
          tokens: 1200,
        },
      }}
    />
  ),
};

/** The run found nothing the queue does not hold already. */
export const DeeperLookNothingMore: StoryObj<typeof MapView> = {
  render: () => (
    <DeeperLookMap
      summary={MAP_ALL_KEPT}
      deeperLook={{
        model: "demo-stub",
        result: {
          provider: "demo",
          model: "demo-stub",
          filed: 0,
          asked: 251,
          blocks: 251,
          tokens: 900,
        },
      }}
    />
  ),
};

/** A long model name, and a run that failed. */
export const DeeperLookFailed: StoryObj<typeof MapView> = {
  render: () => (
    <DeeperLookMap
      deeperLook={{
        model: "claude-sonnet-4-5-20250929",
        error: new Error("anthropic: API error 401: the API key was not accepted"),
      }}
    />
  ),
};
