import type { Meta, StoryObj } from "@storybook/react-vite";
import { BlastRadiusSummary } from "../../brand/BlastRadiusSummary";

const meta: Meta<typeof BlastRadiusSummary> = {
  title: "Brand/BlastRadiusSummary",
  component: BlastRadiusSummary,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 640, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof BlastRadiusSummary>;

/** Promoting this rule would newly flag a chunk of existing content. */
export const NewViolations: Story = {
  args: {
    radius: {
      total_blocks: 240,
      affected_blocks: 22,
      improved_blocks: 0,
      degraded_blocks: 22,
      new_violations: 27,
      resolved_violations: 0,
      critical_count: 3,
      collections: [
        {
          collection_id: "c1",
          collection_name: "Marketing",
          affected_blocks: 15,
          avg_score_delta: -7.2,
        },
        { collection_id: "c2", collection_name: "Docs", affected_blocks: 7, avg_score_delta: -3.1 },
      ],
    },
  },
};

/** A safe rule: nothing currently violates it. */
export const NoImpact: Story = {
  args: {
    radius: {
      total_blocks: 240,
      affected_blocks: 0,
      improved_blocks: 0,
      degraded_blocks: 0,
      new_violations: 0,
      resolved_violations: 0,
      critical_count: 0,
      collections: [],
    },
  },
};
