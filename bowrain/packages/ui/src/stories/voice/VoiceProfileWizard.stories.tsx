import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { VoiceProfileWizard } from "../../voice/VoiceProfileWizard";
import { sampleProfile } from "./fixtures";

const meta: Meta<typeof VoiceProfileWizard> = {
  title: "Brand/VoiceProfileWizard",
  component: VoiceProfileWizard,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 960, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof VoiceProfileWizard>;

/** Start from a blank profile — first step (Identity) shown. */
export const CreateNew: Story = {
  args: {
    onSave: fn(),
    onCancel: fn(),
  },
};

/** Edit an existing profile with all fields pre-filled. */
export const EditExisting: Story = {
  args: {
    profile: sampleProfile,
    onSave: fn(),
    onCancel: fn(),
  },
};
