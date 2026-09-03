import type { Meta, StoryObj } from "@storybook/react-vite";
import { FactGrid } from "./facts";

const meta: Meta = {
  title: "Voice/Facts",
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj;

export const ThreeColumns: Story = {
  name: "Fact grid / Three columns",
  render: () => (
    <div className="max-w-2xl">
      <FactGrid
        facts={[
          { label: "Formality", value: "neutral" },
          { label: "Emotion", value: "measured" },
          { label: "Humor", value: "none" },
        ]}
      />
    </div>
  ),
};

export const FourColumns: Story = {
  name: "Fact grid / Four columns",
  render: () => (
    <div className="max-w-2xl">
      <FactGrid
        columns={4}
        facts={[
          { label: "Voice", value: "active" },
          { label: "Sentences", value: "medium" },
          { label: "Point of view", value: "second" },
          { label: "Contractions", value: "sometimes" },
        ]}
      />
    </div>
  ),
};

export const SkipsEmptyFacts: Story = {
  name: "Fact grid / Skips empty facts",
  render: () => (
    <div className="max-w-2xl">
      <FactGrid
        facts={[
          { label: "Formality", value: "informal" },
          { label: "Emotion", value: undefined },
          { label: "Humor", value: undefined },
        ]}
      />
    </div>
  ),
};
