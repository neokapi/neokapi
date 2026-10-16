import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { SharePanel } from "../components/SharePanel";
import { ShareSetupDialog } from "../components/ShareSetupDialog";
import {
  BOWRAIN_CONNECTED,
  BOWRAIN_WITHOUT_TEAM,
  FOLDER_IN_SYNC,
  FOLDER_UNREACHABLE,
  GIT_IN_SYNC,
  GIT_NEVER_SYNCED,
  GIT_PUSHES_WAITING,
  LOCAL_SHARING,
  S3_IN_SYNC,
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
    sharing: LOCAL_SHARING,
    onSetUp: fn(),
  },
  decorators: [BothThemes],
};

export default meta;
type Story = StoryObj<typeof SharePanel>;

/** On this Mac only: the first rung is current, and its card offers Set up sharing. */
export const Local: Story = {};

/** Shared through git, with nothing waiting either way. The card offers Sync now and Change. */
export const SharedThroughGit: Story = { args: { sharing: GIT_IN_SYNC } };

/** Shared through git, with two operations this Mac has not pushed yet. */
export const PushesWaiting: Story = { args: { sharing: GIT_PUSHES_WAITING } };

/** Shared through git and never synced from this Mac, so what waits to pull is not known. */
export const NeverSynced: Story = { args: { sharing: GIT_NEVER_SYNCED } };

/** Shared through a folder every machine mounts. */
export const SharedThroughFolder: Story = { args: { sharing: FOLDER_IN_SYNC } };

/** Shared through a folder that could not be reached at the last attempt. */
export const Unreachable: Story = { args: { sharing: FOLDER_UNREACHABLE } };

/** Shared through an S3 bucket. */
export const SharedThroughS3: Story = { args: { sharing: S3_IN_SYNC } };

/** Connected to Bowrain: the third rung is current, and its card names the server and workspace. */
export const BowrainConnected: Story = { args: { sharing: BOWRAIN_CONNECTED } };

/** Connected to Bowrain with no team place: the team rung stays to do. */
export const BowrainWithoutTeam: Story = { args: { sharing: BOWRAIN_WITHOUT_TEAM } };

/** A tab over the context alone: the rungs, and why sharing needs a checkout. */
export const ContextOnly: Story = { args: { contextOnly: true } };

/** The dialog Set up sharing and Change open. */
export const SetUpDialog: StoryObj<typeof ShareSetupDialog> = {
  parameters: { oneTheme: true },
  render: () => <ShareSetupDialog tabID="story" open onClose={fn()} onSync={fn()} />,
};
