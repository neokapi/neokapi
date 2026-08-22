import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { fn } from "storybook/test";
import { FocusedReviewer } from "../../components/review/FocusedReviewer";
import type { ReviewEntry } from "../../components/review/reviewQueue";
import type { BlockInfo } from "../../types/api";

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

const Frame = (Story: () => ReactNode) => (
  <div className="flex h-[560px] flex-col overflow-hidden rounded-lg border border-border bg-background">
    {Story()}
  </div>
);

const meta: Meta<typeof FocusedReviewer> = {
  title: "Review/FocusedReviewer",
  component: FocusedReviewer,
  parameters: { layout: "fullscreen" },
  decorators: [Frame],
  args: {
    sourceLocale: "en-US",
    position: { index: 3, total: 12 },
    localeName: (c: string) => (c === "fr-FR" ? "French (France)" : c),
    editing: false,
    onApprove: fn(),
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
