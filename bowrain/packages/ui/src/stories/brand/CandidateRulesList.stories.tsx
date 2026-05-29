import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { CandidateRulesList } from "../../brand/CandidateRulesList";
import type { CandidateRule } from "../../brand/types";

const candidates: CandidateRule[] = [
  {
    term: "utilize",
    replacement: "use",
    correction_count: 7,
    dimension: "vocabulary",
    status: "pending",
  },
  {
    term: "synergy",
    replacement: "teamwork",
    correction_count: 4,
    dimension: "vocabulary",
    status: "pending",
  },
  {
    term: "Globex",
    replacement: "",
    correction_count: 5,
    dimension: "vocabulary",
    status: "approved",
  },
];

const withHistory: CandidateRule[] = [
  ...candidates,
  {
    term: "leverage",
    replacement: "use",
    correction_count: 9,
    dimension: "vocabulary",
    status: "promoted",
    promoted_version: 4,
    auto: true,
  },
  {
    term: "disrupt",
    replacement: "improve",
    correction_count: 3,
    dimension: "vocabulary",
    status: "rejected",
  },
];

const meta: Meta<typeof CandidateRulesList> = {
  title: "Brand/CandidateRulesList",
  component: CandidateRulesList,
  tags: ["autodocs"],
  args: { onPromote: fn(), onReject: fn(), onEvaluate: fn() },
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 720, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof CandidateRulesList>;

/** The review queue: pending and approved candidates awaiting a decision. */
export const ReviewQueue: Story = {
  args: { candidates },
};

/** Full history, including a (auto-)promoted rule and a rejected one. */
export const WithHistory: Story = {
  args: { candidates: withHistory },
};

/** One candidate mid-action (its row is disabled). */
export const Busy: Story = {
  args: { candidates, busyTerm: "utilize" },
};

/** Nothing to review yet. */
export const Empty: Story = {
  args: { candidates: [] },
};
