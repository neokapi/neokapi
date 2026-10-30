import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MapPage, MapView } from "../components/MapPage";
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
