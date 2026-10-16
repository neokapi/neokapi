import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { CandidateReview } from "../../voice/CandidateReview";
import { candidateRows } from "../../voice/candidateRows";
import type { CandidateRule } from "../../voice/types";

const members: Record<string, string> = {
  "u-maria": "Maria Lund",
  "u-tomas": "Tomás Ferreira",
};
const nameOf = (id: string | null | undefined) => (id ? (members[id] ?? id) : "");

const suggested: CandidateRule[] = [
  {
    term: "utilize",
    replacement: "use",
    correction_count: 7,
    dimension: "vocabulary",
    status: "pending",
    concept_id: "c-use",
  },
  {
    term: "synergy",
    replacement: "teamwork",
    correction_count: 4,
    dimension: "vocabulary",
    status: "pending",
  },
  {
    term: "Dear valued customer",
    replacement: "Hello",
    correction_count: 3,
    dimension: "tone",
    status: "pending",
  },
];

const disagreement: CandidateRule[] = [
  {
    term: "log in",
    replacement: "sign in",
    correction_count: 5,
    dimension: "vocabulary",
    status: "pending",
  },
  {
    term: "log in",
    replacement: "log on",
    correction_count: 3,
    dimension: "vocabulary",
    status: "pending",
  },
];

const decided: CandidateRule[] = [
  {
    term: "Globex",
    replacement: "",
    correction_count: 5,
    dimension: "vocabulary",
    status: "approved",
    decided_by: "u-tomas",
    decided_at: "2026-10-14T09:12:00Z",
  },
  {
    term: "leverage",
    replacement: "use",
    correction_count: 9,
    dimension: "vocabulary",
    status: "promoted",
    promoted_version: 4,
    auto: true,
    decided_at: "2026-10-12T16:40:00Z",
  },
  {
    term: "check-out",
    replacement: "checkout",
    correction_count: 6,
    dimension: "vocabulary",
    status: "promoted",
    promoted_version: 3,
    decided_by: "u-maria",
    decided_at: "2026-10-09T11:05:00Z",
  },
  {
    term: "disrupt",
    replacement: "improve",
    correction_count: 3,
    dimension: "vocabulary",
    status: "rejected",
    decided_by: "u-tomas",
    decided_at: "2026-10-08T08:30:00Z",
  },
];

const sample = [...disagreement, ...suggested, ...decided];

const keyOf = (candidates: CandidateRule[], term: string) =>
  candidateRows(candidates).find((r) => r.candidate.term === term)?.key;

const meta: Meta<typeof CandidateReview> = {
  title: "Context/Review/CandidateReview",
  component: CandidateReview,
  tags: ["autodocs"],
  parameters: { layout: "fullscreen" },
  args: {
    candidates: sample,
    onPromote: fn(),
    onReject: fn(),
    onEvaluate: fn(),
    onOpenConcept: fn(),
    nameOf,
    currentUserId: "u-maria",
    // Stories on one docs page would all answer the same key.
    keyboard: false,
  },
  decorators: [
    (Story) => (
      <div style={{ height: 520, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof CandidateReview>;

/** A disagreement, three suggestions, an approved rule and the decided history. */
export const Sample: Story = {
  args: { keyboard: true },
  globals: { theme: "light" },
};

export const SampleDark: Story = {
  args: { keyboard: true },
  globals: { theme: "dark" },
};

/** A suggestion selected: Promote, Change, Reject and Preview impact in the toolbar. */
export const SuggestionSelected: Story = {
  args: { defaultSelected: keyOf(sample, "utilize") },
  globals: { theme: "light" },
};

/** The same, without a project chosen to evaluate against: no Preview impact. */
export const NoProjectChosen: Story = {
  args: { defaultSelected: keyOf(sample, "utilize"), onEvaluate: undefined },
  globals: { theme: "light" },
};

/** A rule promoted by a member, opened from the Established group. */
export const EstablishedSelected: Story = {
  args: { defaultSelected: keyOf(sample, "check-out") },
  globals: { theme: "light" },
};

/** Promoted at the profile's threshold rather than by a person. */
export const AutoPromotedDark: Story = {
  args: { defaultSelected: keyOf(sample, "leverage") },
  globals: { theme: "dark" },
};

/** Accepted by a reviewer and not yet promoted: the platform's own state. */
export const ApprovedSelected: Story = {
  args: { defaultSelected: keyOf(sample, "Globex") },
  globals: { theme: "light" },
};

/** One suggestion and nothing else. */
export const OneSuggestion: Story = {
  args: { candidates: [suggested[0]] },
  globals: { theme: "light" },
};

/** Every suggestion decided: the detail pane says nothing needs you. */
export const NothingWaits: Story = {
  args: { candidates: decided.filter((c) => c.status !== "approved") },
  globals: { theme: "light" },
};

/** Many suggestions: the list scrolls under its group headers. */
export const ManySuggestions: Story = {
  args: {
    candidates: Array.from({ length: 40 }, (_, i) => ({
      term: `phrasing ${i + 1}`,
      replacement: `form ${i + 1}`,
      correction_count: 40 - i,
      dimension: i % 3 === 0 ? ("style" as const) : ("vocabulary" as const),
      status: "pending" as const,
    })),
  },
  globals: { theme: "light" },
};

/** Long phrasings truncate on their row and wrap in the detail pane. */
export const LongContent: Story = {
  args: {
    candidates: [
      {
        term: "we are pleased to inform you that your subscription has been successfully renewed",
        replacement: "your subscription is renewed",
        correction_count: 4,
        dimension: "clarity",
        status: "pending",
      },
      ...suggested,
    ],
  },
  globals: { theme: "light" },
};

/** A decision in flight: its acts are disabled until the server answers. */
export const Busy: Story = {
  args: { defaultSelected: keyOf(sample, "utilize"), busyTerm: "utilize" },
  globals: { theme: "light" },
};

/** A decision the server refused, put back with the reason. */
export const Failed: Story = {
  args: {
    failed: { term: "utilize", error: new Error("forbidden: manage_voice permission required") },
  },
  globals: { theme: "light" },
};

/** Nothing suggested yet. */
export const Empty: Story = {
  args: { candidates: [] },
  globals: { theme: "light" },
};

export const Loading: Story = {
  args: { candidates: null, loading: true },
  globals: { theme: "light" },
};
