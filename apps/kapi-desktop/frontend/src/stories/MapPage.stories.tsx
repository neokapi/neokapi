import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MapView } from "@neokapi/project-screens";
import { MAP_ALL_KEPT, MAP_PENDING } from "@neokapi/project-screens/fixtures";
import { DeeperLookRow, MapPage, type DeeperLookState } from "../components/MapPage";
import { ConnectAICard } from "../components/ConnectAICard";

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
 * The Map in a project tab: the project screens' Map with the parts only the
 * desktop has, the deeper look with AI and adding files. The Map's own states
 * are under Project screens/Map.
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

/** The Map as drawn, with the deeper look's row in a given state. */
function DeeperLookMap({
  state,
  summary = MAP_PENDING,
  connectAI,
}: {
  state: DeeperLookState;
  summary?: typeof MAP_PENDING;
  connectAI?: React.ReactNode;
}) {
  return (
    <MapView
      projectName="Fernwell"
      summary={summary}
      onRun={fn()}
      navigate={fn()}
      deeperLook={<DeeperLookRow state={state} onRun={fn()} />}
      connectAI={connectAI}
    />
  );
}

/** No model connected: the deeper look's button opened the Connect your AI card. */
export const DeeperLookNoModel: Story = {
  render: () => (
    <DeeperLookMap
      state={{ model: null }}
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
export const DeeperLookRunning: Story = {
  render: () => <DeeperLookMap state={{ model: "demo-stub", running: true }} />,
};

/** The run filed three suggestions, which appear as rows; the line says so for a few seconds. */
export const DeeperLookDone: Story = {
  render: () => (
    <DeeperLookMap
      state={{
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
export const DeeperLookNothingMore: Story = {
  render: () => (
    <DeeperLookMap
      summary={MAP_ALL_KEPT}
      state={{
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
export const DeeperLookFailed: Story = {
  render: () => (
    <DeeperLookMap
      state={{
        model: "claude-sonnet-4-5-20250929",
        error: new Error("anthropic: API error 401: the API key was not accepted"),
      }}
    />
  ),
};
