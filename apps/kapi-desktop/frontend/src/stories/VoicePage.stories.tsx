import type { Meta, StoryObj } from "@storybook/react-vite";
import { VoicePage } from "../components/VoicePage";
import { voiceFixture } from "../__tests__/voiceFixture";
import type { ProjectVoiceResult } from "../types/voice";

const meta: Meta<typeof VoicePage> = {
  title: "Pages/Voice",
  component: VoicePage,
  parameters: { layout: "fullscreen" },
  args: { tabID: "t1" },
  decorators: [
    (Story) => (
      <div style={{ height: 720 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof VoicePage>;

/** Several points: one governs, one fell through a closed window, one binds nothing. */
export const ManyPoints: Story = {
  args: { result: voiceFixture },
};

/** The common shape: one point, one profile, and nothing missing. */
export const SinglePoint: Story = {
  args: {
    result: { at: voiceFixture.at, points: [voiceFixture.points[0]] } satisfies ProjectVoiceResult,
  },
};

/** A project that has declared no voice at all. */
export const NothingBound: Story = {
  args: {
    result: {
      at: voiceFixture.at,
      points: [
        {
          label: "project default",
          point: { default: true },
          collections: ["Docs"],
          notes: ["no voice profile binds at this point"],
        },
      ],
    } satisfies ProjectVoiceResult,
  },
};
