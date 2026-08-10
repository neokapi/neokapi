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
    onSuggestBrandRule: fn(),
    onMakeRule: fn(),
    onProposeSourceChange: fn(),
    onEntityPromote: fn(),
  },
};
export default meta;
type Story = StoryObj<typeof FocusedReviewer>;

/** A clean block that passes checks and the on-brand bar. */
export const Clean: Story = {
  args: {
    entry: entry({}),
    onBrand: { rate: 0.94, basis: "voice+checks", onBrandBlocks: 47, translatedBlocks: 50 },
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
    onBrand: { rate: 0.7, basis: "checks", onBrandBlocks: 35, translatedBlocks: 50 },
  },
};

/** With a bound brand profile, the source lane + "make a rule" affordance appear. */
export const WithBrandProfile: Story = {
  args: {
    entry: entry({}),
    brandProfileId: "prof-1",
    onBrand: { rate: 0.88, basis: "voice+checks", onBrandBlocks: 44, translatedBlocks: 50 },
  },
};
