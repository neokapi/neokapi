import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ToneSpectrumSelector } from "../../brand/ToneSpectrumSelector";
import { formalitySpectrum, emotionSpectrum, humorSpectrum } from "../../brand/data/tone-spectrums";

const meta: Meta<typeof ToneSpectrumSelector> = {
  title: "Brand/ToneSpectrumSelector",
  component: ToneSpectrumSelector,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 480, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ToneSpectrumSelector>;

/** Formality spectrum with "neutral" selected. */
export const Formality: Story = {
  args: {
    label: "Formality",
    options: formalitySpectrum,
    value: "neutral",
    onChange: fn(),
  },
};

/** Emotion spectrum with "warm" selected. */
export const Emotion: Story = {
  args: {
    label: "Emotion",
    options: emotionSpectrum,
    value: "warm",
    onChange: fn(),
  },
};

/** Humor spectrum with "none" selected. */
export const Humor: Story = {
  args: {
    label: "Humor",
    options: humorSpectrum,
    value: "none",
    onChange: fn(),
  },
};
