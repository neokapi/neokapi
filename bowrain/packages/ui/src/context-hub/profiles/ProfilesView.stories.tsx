import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ProfilesView } from "./ProfilesView";
import {
  emptyProfiles,
  fragmentedChannels,
  governedButUndeclared,
  populatedProfiles,
  withProfiles,
} from "../../stories/contextProfileFixtures";

const meta: Meta<typeof ProfilesView> = {
  title: "Context/Profiles/ProfilesView",
  component: ProfilesView,
  tags: ["autodocs"],
  parameters: { layout: "fullscreen" },
  args: { onOpenProfile: fn(), onScanVoice: fn() },
  decorators: [
    (Story) => (
      <div style={{ padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ProfilesView>;

/** Several declared points, one of them ungoverned, plus an unbound voice. */
export const Populated: Story = { decorators: [withProfiles(populatedProfiles)] };

/** Nothing governs anything yet: the front door offers both ways in. */
export const Blank: Story = {
  args: { serverUrl: "https://app.bowrain.cloud" },
  decorators: [withProfiles(emptyProfiles)],
};

/** Blank, on a server that runs no scan jobs: the assistant lane alone. */
export const BlankWithoutHostedScan: Story = {
  args: { onScanVoice: undefined, serverUrl: "https://app.bowrain.cloud" },
  decorators: [withProfiles(emptyProfiles)],
};

/** A voice on the default point, but nothing pushed: what would declare more. */
export const OnlyTheDefault: Story = { decorators: [withProfiles(governedButUndeclared)] };

/** No hosted scan configured, so the scan action is absent. */
export const WithoutHostedScan: Story = {
  args: { onScanVoice: undefined },
  decorators: [withProfiles(populatedProfiles)],
};

/**
 * Two projects spell one channel two ways. The workspace says so and judges the
 * pair; neither recipe's slug moves.
 */
export const WithChannelNamesToReconcile: Story = {
  decorators: [withProfiles(populatedProfiles, fragmentedChannels)],
};
