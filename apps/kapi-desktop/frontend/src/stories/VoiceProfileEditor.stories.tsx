import type { Meta, StoryObj } from "@storybook/react-vite";
import { VoiceProfileEditor } from "../components/voice/VoiceProfileEditor";
import { valueSetsFixture, voiceFixture } from "../__tests__/voiceFixture";
import type { VoiceProfile } from "../types/voice";

const profile = voiceFixture.points[0].profile as VoiceProfile;

const meta: Meta<typeof VoiceProfileEditor> = {
  title: "Pages/Voice Editor",
  component: VoiceProfileEditor,
  parameters: { layout: "padded" },
  args: {
    tabID: "t1",
    profileName: "",
    valueSets: valueSetsFixture,
    save: async () => ({ saved: true, changed: true, target: ".kapi/voice.yaml", problems: [] }),
    onSaved: () => {},
    onCancel: () => {},
  },
};

export default meta;
type Story = StoryObj<typeof VoiceProfileEditor>;

/** Editing a profile the recipe already binds. */
export const Editing: Story = {
  args: {
    profile,
    target: { target: ".kapi/voice.yaml", writable: true, exists: true, inherited: false },
  },
};

/** A point with no voice of its own: saving creates one. */
export const Creating: Story = {
  args: {
    profile: { name: "Northsea Support" },
    target: {
      target: ".kapi/profiles/support/voice.yaml",
      writable: true,
      exists: false,
      inherited: true,
    },
  },
};

/** A save the loader refused, with the field that has to change. */
export const Refused: Story = {
  args: {
    profile,
    target: { target: ".kapi/voice.yaml", writable: true, exists: true, inherited: false },
    save: async () => ({
      saved: false,
      changed: false,
      problems: [
        {
          field: "style.person_pov",
          message: 'unknown value "fourth" (expected one of: first_plural, second, third)',
        },
        {
          field: "tone.formality",
          message: '"brisk" is not one of the usual values. It is kept and rendered as written.',
          warning: true,
        },
      ],
    }),
  },
};
