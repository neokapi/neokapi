import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { Button } from "@neokapi/ui-primitives";
import { MapView } from "../MapView";
import { MapScreen } from "../screens";
import type { ProjectScreensPort } from "../port";
import {
  MAP_ALL_KEPT,
  MAP_EMPTY,
  MAP_PARTLY_READ,
  MAP_PENDING,
  MAP_SOURCE_ONLY_UNCHECKED,
  MAP_SUGGESTED_ONLY,
  READER_WITH_FIXES,
  DECIDE_VIEW,
} from "./fixtures";

/**
 * The Map, the page a project opens on: the files, what applies to them, and
 * four counts. A suggestion's Decide opens Decide on that row; a file's Open
 * opens the file in the Reader.
 */
const meta: Meta<typeof MapView> = {
  title: "Project screens/Map",
  component: MapView,
  tags: ["autodocs"],
  args: {
    projectName: "Fernwell",
    summary: MAP_PENDING,
    onRun: fn(),
    navigate: fn(),
    onPreviewFile: fn(),
    addFiles: (
      <Button size="sm" variant="outline">
        Add files
      </Button>
    ),
  },
  parameters: { layout: "fullscreen" },
};

export default meta;
type Story = StoryObj<typeof MapView>;

/** An assistant noticed names, words and a way of writing; one disagreement waits. */
export const SuggestionsPending: Story = {};

/** Suggestions and no disagreement: the Decide button carries the count alone. */
export const SuggestionsOnly: Story = { args: { summary: MAP_SUGGESTED_ONLY } };

/** kapi read fewer files than the project has, so the line under the title names how many. */
export const PartlyRead: Story = { args: { summary: MAP_PARTLY_READ } };

/** A new project whose collections match no file yet. */
export const Empty: Story = { args: { projectName: "Untitled", summary: MAP_EMPTY } };

/** Every suggestion kept: the rules hold and no file has a finding. */
export const AllKept: Story = { args: { summary: MAP_ALL_KEPT } };

/** A source-only project not checked yet. */
export const NotCheckedYet: Story = { args: { summary: MAP_SOURCE_ONLY_UNCHECKED } };

/** A checks run over the whole project in progress. */
export const Checking: Story = { args: { running: true } };

/** A port with the reads alone: no check action, and nothing the host does not open. */
const READ_ONLY: ProjectScreensPort = {
  mapSummary: async () => MAP_PENDING,
  readerFile: async () => READER_WITH_FIXES,
  decideView: async () => DECIDE_VIEW,
};

/** A read-only preview: drawn through a port with no actions, opening only the Reader. */
export const ReadOnly: StoryObj<typeof MapScreen> = {
  render: () => (
    <MapScreen
      port={READ_ONLY}
      projectName="Fernwell"
      navigate={fn()}
      canNavigate={(target) => target.kind === "reader"}
    />
  ),
};
