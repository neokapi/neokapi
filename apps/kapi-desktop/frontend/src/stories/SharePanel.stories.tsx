import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { SharePanel } from "../components/SharePanel";
import { ShareSetupDialog } from "../components/ShareSetupDialog";
import {
  BOWRAIN_VIEW,
  BOWRAIN_WITHOUT_TEAM_VIEW,
  BROKEN_PLACE_VIEW,
  FOLDER_LONG_PATH_VIEW,
  FOLDER_UNREACHABLE_VIEW,
  FOLDER_VIEW,
  GIT_NEVER_SYNCED_VIEW,
  GIT_PUSHES_WAITING_VIEW,
  GIT_VIEW,
  LOCAL_VIEW,
  LOCAL_VIEW_WITH_PLUGIN,
  S3_VIEW,
} from "./fixtures/sharing";

/**
 * The panel at the hub's reading width, once in each theme. A story that opens
 * a dialog sets `oneTheme`, since a modal drawn twice covers both copies.
 */
const BothThemes: Decorator = (Story, { parameters }) => {
  if (parameters.oneTheme) return <Story />;
  return (
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
};

const meta: Meta<typeof SharePanel> = {
  title: "Components/SharePanel",
  component: SharePanel,
  tags: ["autodocs"],
  args: {
    tabID: "story",
    projectName: "Fernwell",
    view: LOCAL_VIEW,
    onSetUp: fn(),
  },
  decorators: [BothThemes],
};

export default meta;
type Story = StoryObj<typeof SharePanel>;

/** On this Mac: the first rung is current, with the workspace and Change. */
export const OnThisMac: Story = {};

/** On this Mac because kapi.yaml names a place that cannot be read: the error, and Change to fix it. */
export const BrokenPlace: Story = { args: { view: BROKEN_PLACE_VIEW } };

/** On this Mac, with the kapi-bowrain plugin installed: Bowrain's rung names the command alone. */
export const PluginInstalled: Story = { args: { view: LOCAL_VIEW_WITH_PLUGIN } };

/** With your team through git, nothing waiting either way: Sync now and Change. */
export const ThroughGit: Story = { args: { view: GIT_VIEW } };

/** Through git, with two operations this Mac has not pushed yet. */
export const PushesWaiting: Story = { args: { view: GIT_PUSHES_WAITING_VIEW } };

/** Through git and never synced from this Mac, so what waits to pull is not known. */
export const NeverSynced: Story = { args: { view: GIT_NEVER_SYNCED_VIEW } };

/** Through a folder every machine mounts. */
export const ThroughAFolder: Story = { args: { view: FOLDER_VIEW } };

/** Through a folder with a long path, which truncates in its chip. */
export const LongFolderPath: Story = { args: { view: FOLDER_LONG_PATH_VIEW } };

/** Through a folder that could not be reached at the last attempt. */
export const Unreachable: Story = { args: { view: FOLDER_UNREACHABLE_VIEW } };

/** Through an S3 bucket. */
export const ThroughS3: Story = { args: { view: S3_VIEW } };

/** Managed on Bowrain: the third rung is current, with the server and workspace; the team place stays under its rung. */
export const ManagedOnBowrain: Story = { args: { view: BOWRAIN_VIEW } };

/** Managed on Bowrain with no team place: the team rung offers Change. */
export const BowrainWithoutTeam: Story = { args: { view: BOWRAIN_WITHOUT_TEAM_VIEW } };

/** A tab over the context alone: the rungs, and why sharing needs a checkout. */
export const ContextOnly: Story = { args: { contextOnly: true } };

/** The dialog Change opens. */
export const ChangeDialog: StoryObj<typeof ShareSetupDialog> = {
  parameters: { oneTheme: true },
  render: () => <ShareSetupDialog tabID="story" open onClose={fn()} onSync={fn()} />,
};
