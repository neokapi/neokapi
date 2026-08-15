import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ProfilesView } from "./ProfilesView";
import {
  emptyProfiles,
  fragmentedChannels,
  populatedProfiles,
  withProfiles,
} from "../../stories/contextProfileFixtures";

const meta: Meta<typeof ProfilesView> = {
  title: "Brand Hub/Profiles/ProfilesView",
  component: ProfilesView,
  tags: ["autodocs"],
  parameters: { layout: "fullscreen" },
  args: { onOpenProfile: fn(), onScanBrand: fn() },
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

/** A workspace that has pushed nothing: one point, and what would declare more. */
export const OnlyTheDefault: Story = { decorators: [withProfiles(emptyProfiles)] };

/** No hosted scan configured, so the scan action is absent. */
export const WithoutHostedScan: Story = {
  args: { onScanBrand: undefined },
  decorators: [withProfiles(populatedProfiles)],
};

/**
 * Two projects spell one channel two ways. The workspace says so and judges the
 * pair; neither recipe's slug moves.
 */
export const WithChannelNamesToReconcile: Story = {
  decorators: [withProfiles(populatedProfiles, fragmentedChannels)],
};
