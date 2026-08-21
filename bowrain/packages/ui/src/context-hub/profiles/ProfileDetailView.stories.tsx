import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ProfileDetailView } from "./ProfileDetailView";
import {
  emptyProfiles,
  populatedProfiles,
  withProfiles,
} from "../../stories/contextProfileFixtures";

const meta: Meta<typeof ProfileDetailView> = {
  title: "Context/Profiles/ProfileDetailView",
  component: ProfileDetailView,
  tags: ["autodocs"],
  parameters: { layout: "fullscreen" },
  args: {
    onBack: fn(),
    onOpenVoice: fn(),
    onOpenTerms: fn(),
    onOpenChanges: fn(),
    onScanBrand: fn(),
  },
  decorators: [
    (Story) => (
      <div style={{ padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ProfileDetailView>;

/** A declared point with a voice, two collections, and changes in review. */
export const DeclaredPoint: Story = {
  args: { slug: "channel~docs.product~bowrain" },
  decorators: [withProfiles(populatedProfiles)],
};

/** A point content sits at with no voice bound to it. */
export const WithoutVoice: Story = {
  args: { slug: "channel~app.product~bowrain" },
  decorators: [withProfiles(populatedProfiles)],
};

/** The workspace's own point, which carries the workspace-wide scan. */
export const DefaultProfile: Story = {
  args: { slug: "default" },
  decorators: [withProfiles(populatedProfiles)],
};

/** The default point before anything has been pushed or scanned. */
export const DefaultProfileEmpty: Story = {
  args: { slug: "default" },
  decorators: [withProfiles(emptyProfiles)],
};

/** A voice the workspace holds that governs nothing. */
export const UnboundVoice: Story = {
  args: { slug: "voice~v-support" },
  decorators: [withProfiles(populatedProfiles)],
};

/** A slug no profile answers to. */
export const NotFound: Story = {
  args: { slug: "channel~print" },
  decorators: [withProfiles(populatedProfiles)],
};
