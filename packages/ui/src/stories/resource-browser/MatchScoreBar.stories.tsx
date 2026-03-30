import type { Meta, StoryObj } from "@storybook/react-vite";
import { MatchScoreBar } from "../../components/resource-browser/MatchScoreBar";

const meta: Meta<typeof MatchScoreBar> = {
  title: "Resource Browser/MatchScoreBar",
  component: MatchScoreBar,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ width: 300, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
  parameters: {
    docs: {
      description: {
        component:
          "Horizontal bar visualizing a TM match score (0-1.0). Color coded: red < 0.7, amber 0.7-0.85, green 0.85-0.99, blue 1.0.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof MatchScoreBar>;

export const ExactMatch: Story = {
  args: {
    score: 1.0,
    matchType: "exact",
  },
};

export const HighFuzzy: Story = {
  args: {
    score: 0.92,
    matchType: "fuzzy",
  },
};

export const MediumFuzzy: Story = {
  args: {
    score: 0.75,
    matchType: "fuzzy",
  },
};

export const LowFuzzy: Story = {
  args: {
    score: 0.55,
    matchType: "fuzzy",
  },
};

export const GeneralizedExact: Story = {
  args: {
    score: 1.0,
    matchType: "generalized-exact",
  },
};

export const StructuralFuzzy: Story = {
  args: {
    score: 0.88,
    matchType: "structural-fuzzy",
  },
};

/** All match quality levels compared side by side. */
export const AllScores: Story = {
  render: () => (
    <div className="space-y-3">
      <MatchScoreBar score={1.0} matchType="exact" />
      <MatchScoreBar score={1.0} matchType="generalized-exact" />
      <MatchScoreBar score={0.92} matchType="fuzzy" />
      <MatchScoreBar score={0.88} matchType="structural-fuzzy" />
      <MatchScoreBar score={0.75} matchType="fuzzy" />
      <MatchScoreBar score={0.55} matchType="fuzzy" />
    </div>
  ),
};
