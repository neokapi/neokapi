import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { VoiceProfileEditor } from "../../voice/VoiceProfileEditor";
import { sampleProfile } from "./fixtures";

const meta: Meta<typeof VoiceProfileEditor> = {
  title: "Brand/VoiceProfileEditor",
  component: VoiceProfileEditor,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 720, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof VoiceProfileEditor>;

/** Create a new profile from scratch. */
export const CreateNew: Story = {
  args: {
    onSave: fn(),
    onCancel: fn(),
  },
};

/** Edit an existing profile with pre-filled data. */
export const EditExisting: Story = {
  args: {
    profile: sampleProfile,
    onSave: fn(),
    onCancel: fn(),
  },
};
