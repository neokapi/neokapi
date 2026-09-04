import type { Meta, StoryObj } from "@storybook/react-vite";
import { ProvenanceCard } from "../../components/review";

const meta: Meta<typeof ProvenanceCard> = {
  title: "Review/ProvenanceCard",
  component: ProvenanceCard,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "Where this translation came from, and the decision in force over it. One decision per unit and variant, overwritten by the next, so this is the decision that stands. A decision recorded against source wording that has since changed is marked stale.",
      },
    },
  },
  render: (args) => (
    <div className="w-[28rem]">
      <ProvenanceCard {...args} />
    </div>
  ),
};

export default meta;
type Story = StoryObj<typeof ProvenanceCard>;

export const RejectedStale: Story = {
  name: "AI translation, rejected on wording that has since changed",
  args: {
    provenance: {
      origin: { kind: "ai", engine: "claude-sonnet", tool: "translate" },
      review_state: "rejected",
      status: "draft",
      by: "maria@bowrain.test",
      at: "2026-08-30T09:12:00Z",
      note: "Reads as machine output; soften the imperative.",
      stale: true,
    },
  },
};

export const SignedOff: Story = {
  name: "Recycled from content memory, signed off",
  args: {
    provenance: {
      origin: { kind: "memory", timestamp: "2026-08-29T18:40:00Z" },
      review_state: "signed-off",
      status: "signed-off",
      by: "sam@bowrain.test",
      at: "2026-08-31T08:00:00Z",
    },
  },
};

export const Undecided: Story = {
  name: "Written by a person, no decision yet",
  args: {
    provenance: {
      origin: { kind: "human", timestamp: "2026-08-29T18:40:00Z" },
    },
  },
};

export const WithUnitNote: Story = {
  name: "A note kept beside the unit (platform block note)",
  args: {
    provenance: { origin: { kind: "human", timestamp: "2026-08-29T18:40:00Z" } },
    note: "Legal asked us to keep the product name unchanged.",
  },
};

export const Empty: Story = {
  name: "Nothing recorded",
  args: { provenance: {} },
};

export const Dark: Story = {
  globals: { theme: "dark" },
  args: {
    provenance: {
      origin: { kind: "ai", engine: "claude-sonnet", tool: "translate" },
      review_state: "rejected",
      status: "draft",
      by: "maria@bowrain.test",
      at: "2026-08-30T09:12:00Z",
      note: "Reads as machine output; soften the imperative.",
      stale: true,
    },
  },
};
