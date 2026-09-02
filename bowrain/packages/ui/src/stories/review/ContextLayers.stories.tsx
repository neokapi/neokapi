import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  ContextLayer,
  FindingsList,
  MemoryMatchCard,
  NeighbourCell,
  PointRail,
  ProvenanceBlock,
  findingsSummary,
  memorySummary,
  neighbourhoodSummary,
  provenanceSummary,
} from "../../components/review/reviewContext";
import type { CheckIssue, ReviewContext } from "../../types/api";
import type { VoiceFinding } from "../../voice/types";
import { sampleReviewVoiceProfile } from "../fixtures";

const issues: CheckIssue[] = [
  {
    type: "placeholder",
    severity: "error",
    message: "Target is missing the {name} placeholder.",
    original_text: "{name}",
    suggestion: "Réinitialisez votre {name}",
  },
];

const findings: VoiceFinding[] = [
  {
    category: "compliance",
    severity: "major",
    message: "Uses a term the profile forbids.",
    original_text: "Réinitialisez",
    suggestion: "Changez",
  },
];

/** Everything the server resolved for one unit under review. */
const context: ReviewContext = {
  block_id: "b1",
  item_name: "auth.json",
  locale: "fr-FR",
  voice_profile: sampleReviewVoiceProfile,
  terms: [
    {
      source_term: "password",
      target_terms: ["mot de passe"],
      domain: "account",
      status: "preferred",
      start: 11,
      end: 19,
    },
  ],
  collection_id: "col-1",
  collection_name: "Product UI",
  coordinates: { product: "kapi", channel: "app", brand: "bowrain" },
  previous: {
    block_id: "b0",
    source_runs: [{ text: "Enter the email you signed up with" }],
    status: "reviewed",
  },
  next: {
    block_id: "b2",
    source_runs: [
      { text: "We sent a link to " },
      { ph: { id: "1", type: "code:variable", data: "{{.Email}}", equiv: "{{.Email}}" } },
      { text: "." },
    ],
  },
  memory_match: {
    source: "Reset your password",
    target: "Réinitialisez votre mot de passe",
    score: 0.92,
    match_type: "fuzzy",
  },
  decision: {
    state: "rejected",
    status: "draft",
    by: "maria@bowrain.test",
    at: "2026-08-30T09:12:00Z",
    note: "Reads as machine output; soften the imperative.",
    source_moved: true,
  },
  notes: [],
  voice_findings: findings,
  voice_score: 62,
  voice_bar: 90,
  origin: {
    kind: "ai",
    engine: "claude-sonnet",
    tool: "translate",
    timestamp: "2026-08-29T18:40:00Z",
    profile: "vp-1",
  },
};

/**
 * The five layers a reviewer decides in, stacked as a rail. `open` draws the
 * evidence under each summary; closed leaves the summary line alone, which is
 * the state a reviewer scans before opening the one they need.
 */
function Layers({ open }: { open: boolean }) {
  return (
    <div className="w-[26rem] space-y-4 rounded-lg border border-border bg-background p-4">
      <PointRail context={context} />
      <ContextLayer
        title="Neighbourhood"
        summary={neighbourhoodSummary(context)}
        defaultOpen={open}
      >
        <NeighbourCell neighbour={context.previous} where="previous" />
        <NeighbourCell neighbour={context.next} where="next" />
      </ContextLayer>
      <ContextLayer title="Findings" summary={findingsSummary(issues, findings)} defaultOpen={open}>
        <FindingsList issues={issues} findings={findings} />
      </ContextLayer>
      <ContextLayer
        title="Content memory"
        summary={memorySummary(context.memory_match)}
        defaultOpen={open}
      >
        <MemoryMatchCard match={context.memory_match} />
      </ContextLayer>
      <ContextLayer
        title="Provenance"
        summary={provenanceSummary(context.origin, context.decision)}
        defaultOpen={open}
      >
        <ProvenanceBlock origin={context.origin} decision={context.decision} />
      </ContextLayer>
    </div>
  );
}

const meta: Meta<typeof Layers> = {
  title: "Review/ContextLayers",
  component: Layers,
  parameters: { layout: "centered" },
};
export default meta;
type Story = StoryObj<typeof Layers>;

/**
 * Every layer answered on one line: the coordinates the unit sits at, how many
 * neighbours it has, what the checks found, what the corpus already said, and
 * where the target came from. Five lines, and no layer hidden.
 */
export const Collapsed: Story = { render: () => <Layers open={false} /> };

/**
 * The same five with their evidence open. Bound context reads neutral, so the
 * profile, its term rules (the do-not-translate one carrying a lock), the
 * matched terms, the coordinates and the memory match all take the default
 * border and muted text, with the bite of a rule in its tooltip. Red belongs to
 * the findings, where a violation has actually been named.
 */
export const Expanded: Story = { render: () => <Layers open /> };
