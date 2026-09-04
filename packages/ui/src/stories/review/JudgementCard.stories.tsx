import type { Meta, StoryObj } from "@storybook/react-vite";
import { JudgementCard, type ReviewFindingView } from "../../components/review";

const findings: ReviewFindingView[] = [
  {
    id: "finding-issue-0",
    category: "placeholder",
    severity: "error",
    tone: "destructive",
    message: "Target is missing the {name} placeholder.",
    originalText: "{name}",
    suggestion: "Réinitialisez votre {name}",
  },
  {
    id: "finding-voice-0",
    category: "compliance",
    severity: "major",
    tone: "destructive",
    message: "Uses a term the profile forbids.",
    originalText: "Réinitialisez",
    suggestion: "Changez",
  },
  {
    id: "finding-voice-1",
    category: "style",
    severity: "minor",
    tone: "warning",
    message: "Prefers the second person here.",
  },
  {
    id: "finding-source-0",
    category: "voice",
    severity: "major",
    tone: "destructive",
    message: 'Forbidden term "cart".',
    field: "source",
  },
];

const meta: Meta<typeof JudgementCard> = {
  title: "Review/JudgementCard",
  component: JudgementCard,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "What has already been said about this translation: every checker's findings as one list, painted on the shared severity scale, with what each was raised against and what to say instead; and the AI pre-review that scored it, inside the same card. A source-side finding is marked so it does not read as a defect in the translation.",
      },
    },
  },
  render: (args) => (
    <div className="w-[28rem]">
      <JudgementCard {...args} />
    </div>
  ),
};

export default meta;
type Story = StoryObj<typeof JudgementCard>;

export const WithFindings: Story = {
  name: "Findings from both checkers, AI pre-review absent",
  args: { findings },
};

export const WithAIPreReview: Story = {
  name: "Clean, with an AI pre-review present",
  args: {
    findings: [],
    aiScore: 84,
    aiModel: "claude-sonnet",
    aiFindings: [
      {
        severity: "minor",
        message: "Slightly formal for the surface.",
        suggestion: "On continue ?",
      },
    ],
  },
};

export const FindingsAndAI: Story = {
  name: "Findings and an AI pre-review",
  args: { findings: findings.slice(0, 2), aiScore: 41, aiModel: "claude-sonnet" },
};

export const WithSurfaceControls: Story = {
  name: "The surface's own re-check above the list",
  args: {
    findings: findings.slice(1, 3),
    children: (
      <button type="button" className="rounded border px-2 py-0.5 text-xs">
        Re-check
      </button>
    ),
  },
};

export const Loading: Story = {
  name: "Unit still loading",
  args: {},
};

export const Dark: Story = {
  globals: { theme: "dark" },
  args: { findings, aiScore: 41, aiModel: "claude-sonnet" },
};
