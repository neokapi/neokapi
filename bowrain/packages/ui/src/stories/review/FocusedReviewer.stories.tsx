import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { fn } from "storybook/test";
import { FocusedReviewer } from "../../components/review/FocusedReviewer";
import type { ReviewEntry } from "../../components/review/reviewQueue";
import type { BlockInfo, ReviewContext } from "../../types/api";
import { sampleReviewVoiceProfile } from "../fixtures";

function block(over: Partial<BlockInfo>): BlockInfo {
  return {
    id: "b1",
    source: "Reset your password",
    source_coded: "Reset your password",
    source_spans: [],
    targets: { "fr-FR": { text: "Réinitialisez votre mot de passe", status: "translated" } },
    translatable: true,
    has_spans: false,
    properties: {},
    entities: [
      { key: "entity:0", text: "password", type: "entity:product", start: 11, end: 19, dnt: false },
    ],
    ...over,
  };
}

function entry(over: Partial<ReviewEntry>): ReviewEntry {
  return {
    id: "itm-1::b1::fr-FR",
    itemId: "itm-1",
    itemName: "auth.json",
    // The server names the collection on every queue entry; "" is the item
    // that belongs to none, and "" terminology is a target no governance was
    // active for — not one that cleared it.
    collectionId: "",
    termCompliance: "",
    locale: "fr-FR",
    block: block({}),
    issues: [],
    ...over,
  };
}

// Tall enough to read the unit and the context rail beside it without
// scrolling the pane, which is what the reviewer's own window gives them.
const Frame = (Story: () => ReactNode) => (
  <div className="flex h-[860px] flex-col overflow-hidden rounded-lg border border-border bg-background">
    {Story()}
  </div>
);

/**
 * The unit under decision with everything the server resolved for it: what
 * governs the point, what sits either side of it, what the corpus and the
 * ledger already said, what the scoring pass found, and how the target was
 * produced.
 */
const resolvedContext: ReviewContext = {
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
  coordinates: { product: "kapi", channel: "app" },
  previous: {
    block_id: "b0",
    source_runs: [{ text: "Enter the email you signed up with" }],
    target_runs: [{ text: "Saisissez l'adresse e-mail utilisée à l'inscription" }],
    status: "reviewed",
  },
  next: {
    block_id: "b2",
    source_runs: [
      { text: "We sent a link to " },
      { ph: { id: "1", type: "code:variable", data: "{{.Email}}", equiv: "{{.Email}}" } },
      { text: "." },
    ],
    target_runs: [
      { text: "Nous avons envoyé un lien à " },
      { ph: { id: "1", type: "code:variable", data: "{{.Email}}", equiv: "{{.Email}}" } },
      { text: "." },
    ],
    status: "draft",
  },
  memory_match: {
    source: "Reset your password",
    target: "Réinitialisez votre mot de passe",
    score: 1,
    match_type: "exact",
  },
  decision: {
    state: "rejected",
    status: "draft",
    by: "maria@bowrain.test",
    at: "2026-08-30T09:12:00Z",
    note: "Reads as machine output; soften the imperative.",
    source_moved: true,
  },
  notes: [
    {
      id: "note-1",
      blockId: "b1",
      author: "maria@bowrain.test",
      text: "Legal asked us to keep the product name unchanged.",
      createdAt: "2026-08-30T09:15:00Z",
    },
  ],
  voice_findings: [
    {
      category: "compliance",
      severity: "major",
      message: "Uses a term the profile forbids.",
      original_text: "Réinitialisez",
      suggestion: "Changez",
      position: { kind: "range", start: { run: 0, offset: 0 }, end: { run: 0, offset: 13 } },
    },
  ],
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
 * The same five layers for a unit that clears every bar: the score sits above
 * the profile's bar, so the point rail reads it in muted ink and the findings
 * list is empty. A verdict and the context beside it always agree.
 */
const passingContext: ReviewContext = {
  ...resolvedContext,
  decision: undefined,
  voice_findings: [],
  voice_score: 94,
  voice_bar: 80,
};

const meta: Meta<typeof FocusedReviewer> = {
  title: "Review/FocusedReviewer",
  component: FocusedReviewer,
  parameters: { layout: "fullscreen" },
  decorators: [Frame],
  args: {
    sourceLocale: "en-US",
    position: { index: 3, total: 12 },
    localeName: (c: string) => (c === "fr-FR" ? "French (France)" : "English (United States)"),
    editing: false,
    onApprove: fn(),
    onSignOff: fn(),
    onReject: fn(),
    onEditToggle: fn(),
    onSaveEdit: fn(),
    onCancelEdit: fn(),
    onReCheck: fn(),
    onMarkTerm: fn(),
    onSuggestVoiceRule: fn(),
    onMakeRule: fn(),
    onProposeSourceChange: fn(),
    onEntityPromote: fn(),
  },
};
export default meta;
type Story = StoryObj<typeof FocusedReviewer>;

/** A block that clears every bar the server applies on approve. */
export const Passing: Story = {
  args: {
    entry: entry({ termCompliance: "compliant", voiceScore: 94, voiceBar: 80 }),
    context: passingContext,
    compliance: { rate: 0.94, basis: "voice+checks", compliantBlocks: 47, translatedBlocks: 50 },
  },
};

/**
 * Checks pass, but the target uses a forbidden term — the server refuses it,
 * and the reviewer sees which bar it missed rather than a green chip.
 */
export const TermViolation: Story = {
  args: {
    entry: entry({ termCompliance: "violation", voiceScore: 91, voiceBar: 80 }),
    compliance: {
      rate: 0.81,
      basis: "voice+checks+terms",
      compliantBlocks: 40,
      translatedBlocks: 50,
    },
  },
};

/** Checks and terminology pass; the voice score sits below the profile's bar. */
export const BelowVoiceBar: Story = {
  args: {
    entry: entry({ termCompliance: "compliant", voiceScore: 62, voiceBar: 90 }),
    context: resolvedContext,
    compliance: {
      rate: 0.66,
      basis: "voice+checks+terms",
      compliantBlocks: 33,
      translatedBlocks: 50,
    },
  },
};

/** A block flagged by failing checks — the reviewer sees why. */
export const FailingChecks: Story = {
  args: {
    entry: entry({
      issues: [
        {
          type: "placeholder",
          severity: "error",
          message: "Target is missing the {name} placeholder.",
        },
        { type: "length", severity: "warning", message: "Target is much longer than the source." },
      ],
    }),
    compliance: { rate: 0.7, basis: "checks", compliantBlocks: 35, translatedBlocks: 50 },
  },
};

/**
 * A block carrying inline codes — a variable inside a bold pair, as an email
 * catalog holds it. Both cells are the same primitive, so the placeholder reads
 * as the same chip on either side and the two texts differ only where the
 * translation does.
 */
export const InlineCodes: Story = {
  args: {
    entry: entry({
      block: block({
        source: "Your credits reset on . Upgrade any time.",
        source_coded: "Your credits reset on \uE001\uE003\uE002. Upgrade any time.",
        source_spans: [
          { span_type: "opening", type: "fmt:bold", id: "1", data: "<strong>" },
          {
            span_type: "placeholder",
            type: "code:variable",
            id: "2",
            data: "{{.ResetDate}}",
            equiv_text: "{{.ResetDate}}",
          },
          { span_type: "closing", type: "fmt:bold", id: "1", data: "</strong>" },
        ],
        has_spans: true,
        entities: [],
        targets: {
          "fr-FR": {
            text: "Vos crédits sont réinitialisés le . Améliorez à tout moment.",
            status: "translated",
          },
        },
        targets_coded: {
          "fr-FR": "Vos crédits sont réinitialisés le \uE001\uE003\uE002. Améliorez à tout moment.",
        },
      }),
      termCompliance: "compliant",
      voiceScore: 90,
      voiceBar: 80,
    }),
  },
};

/** With a bound voice profile, the source lane + "make a rule" affordance appear. */
export const WithVoiceProfile: Story = {
  args: {
    entry: entry({}),
    voiceProfileId: "prof-1",
    compliance: { rate: 0.88, basis: "voice+checks", compliantBlocks: 44, translatedBlocks: 50 },
  },
};

/**
 * All five layers populated: the point rail names the profile and its guidance,
 * the neighbours sit above and below the unit, the findings carry their anchor
 * and their suggestion, the content-memory match shows both sides with a
 * one-click use, and the provenance block says who decided what, when.
 */
export const WithResolvedContext: Story = {
  args: {
    entry: entry({
      termCompliance: "violation",
      voiceScore: 62,
      voiceBar: 90,
      issues: [
        {
          type: "placeholder",
          severity: "error",
          message: "Target is missing the {name} placeholder.",
          original_text: "{name}",
          suggestion: "Réinitialisez votre {name}",
        },
      ],
    }),
    voiceProfileId: "vp-1",
    context: resolvedContext,
    compliance: {
      rate: 0.66,
      basis: "voice+checks+terms",
      compliantBlocks: 33,
      translatedBlocks: 50,
    },
  },
};

/**
 * The same layout with nothing resolved: no profile bound, no terms matched, no
 * neighbours, no content-memory match, no decision recorded. Each layer says
 * which of those it is, so an ungoverned unit does not read as a broken panel.
 */
export const WithoutResolvedContext: Story = {
  args: {
    entry: entry({}),
    context: {
      block_id: "b1",
      item_name: "auth.json",
      locale: "fr-FR",
      terms: [],
      collection_id: "",
      notes: [],
      voice_findings: [],
    },
  },
};
