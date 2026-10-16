import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { SharePanel } from "../components/SharePanel";
import { ShareSetupDialog } from "../components/ShareSetupDialog";
import {
  BOWRAIN_CONNECTED,
  FOLDER_UNREACHABLE,
  GIT_IN_SYNC,
  GIT_NEVER_SYNCED,
  GIT_PUSHES_WAITING,
  LOCAL_SHARING,
  S3_IN_SYNC,
} from "./fixtures/sharing";

const meta: Meta<typeof SharePanel> = {
  title: "Components/SharePanel",
  component: SharePanel,
  tags: ["autodocs"],
  args: {
    tabID: "story",
    sharing: LOCAL_SHARING,
    onSetUp: fn(),
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
type Story = StoryObj<typeof SharePanel>;

/** On this Mac: kapi.yaml names no place, so the first rung is marked and the second offers Set up sharing. */
export const Local: Story = {};

/** Shared through git, with nothing waiting either way. */
export const SharedInSync: Story = { args: { sharing: GIT_IN_SYNC } };

/** Shared through git, with two operations this Mac has not pushed yet. */
export const PushesWaiting: Story = { args: { sharing: GIT_PUSHES_WAITING } };

/** Shared through git and never synced from this Mac, so what waits to pull is not known. */
export const NeverSynced: Story = { args: { sharing: GIT_NEVER_SYNCED } };

/** Shared through a folder that could not be reached at the last attempt. */
export const Unreachable: Story = { args: { sharing: FOLDER_UNREACHABLE } };

/** Shared through an S3 bucket. */
export const SharedThroughS3: Story = { args: { sharing: S3_IN_SYNC } };

/** Connected to Bowrain: the third rung is marked. */
export const BowrainConnected: Story = { args: { sharing: BOWRAIN_CONNECTED } };

/** A tab over the context alone: the first rung, and the line saying sharing needs a checkout. */
export const ContextOnly: Story = { args: { contextOnly: true } };

/** The dialog Set up sharing opens. */
export const SetUpDialog: StoryObj<typeof ShareSetupDialog> = {
  render: () => <ShareSetupDialog tabID="story" open onClose={fn()} onSync={fn()} />,
};
