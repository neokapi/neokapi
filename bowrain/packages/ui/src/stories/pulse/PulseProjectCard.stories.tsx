import type { Meta, StoryObj } from "@storybook/react";
import { PulseProjectCard } from "../../components/pulse";
import { mockProjects } from "./pulse-fixtures";

const meta: Meta<typeof PulseProjectCard> = {
  title: "Pulse/PulseProjectCard",
  component: PulseProjectCard,
  tags: ["autodocs"],
  parameters: { layout: "padded" },
};

export default meta;
type Story = StoryObj<typeof PulseProjectCard>;

const p = mockProjects[0];

export const Default: Story = {
  args: {
    name: p.name,
    sourceLanguage: p.source_language,
    targetLanguages: p.target_languages,
    totalWords: p.total_words,
    translatedWords: p.translated_words,
    percentage: p.percentage,
  },
};

export const HighProgress: Story = {
  args: {
    name: p.name,
    sourceLanguage: p.source_language,
    targetLanguages: p.target_languages,
    totalWords: p.total_words,
    translatedWords: p.total_words * 0.95,
    percentage: 95,
  },
};

export const LowProgress: Story = {
  args: {
    name: p.name,
    sourceLanguage: p.source_language,
    targetLanguages: p.target_languages,
    totalWords: p.total_words,
    translatedWords: p.total_words * 0.15,
    percentage: 15,
  },
};
