import type { Meta, StoryObj } from "@storybook/react-vite";
import { BrandVoicePreview } from "../../brand/BrandVoicePreview";

const meta: Meta<typeof BrandVoicePreview> = {
  title: "Brand/BrandVoicePreview",
  component: BrandVoicePreview,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 320, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof BrandVoicePreview>;

/** Casual, warm voice with contractions and first-person plural. */
export const CasualWarm: Story = {
  args: {
    tone: {
      personality: ["friendly", "approachable", "enthusiastic"],
      formality: "casual",
      emotion: "warm",
      humor: "light",
    },
    style: {
      active_voice: true,
      sentence_length: "varied",
      person_pov: "first_plural",
      contractions: "always",
    },
  },
};

/** Formal, authoritative voice with no contractions and second-person. */
export const FormalAuthoritative: Story = {
  args: {
    tone: {
      personality: ["professional", "precise", "trustworthy"],
      formality: "formal",
      emotion: "authoritative",
      humor: "none",
    },
    style: {
      active_voice: true,
      sentence_length: "medium",
      person_pov: "second",
      contractions: "never",
    },
  },
};

/** Technical, neutral voice with short sentences and third-person. */
export const TechnicalNeutral: Story = {
  args: {
    tone: {
      personality: ["concise", "technical", "direct"],
      formality: "technical",
      emotion: "neutral",
      humor: "none",
    },
    style: {
      active_voice: true,
      sentence_length: "short",
      person_pov: "third",
      contractions: "sometimes",
    },
  },
};
