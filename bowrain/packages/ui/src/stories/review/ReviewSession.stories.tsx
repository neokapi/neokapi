import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { ReviewSession } from "../../components/review/ReviewSession";
import { createProvidersDecorator } from "../decorators";
import { emptyReviewContext, sampleProject } from "../fixtures";
import type { BlockInfo, TranslationDashboardStats } from "../../types/api";

const blocks: BlockInfo[] = [
  {
    id: "b1",
    source: "Welcome to your dashboard",
    source_coded: "Welcome to your dashboard",
    source_spans: [],
    targets: { "fr-FR": { text: "Bienvenue sur votre tableau de bord", status: "translated" } },
    translatable: true,
    has_spans: false,
    properties: {},
  },
  {
    id: "b2",
    source: "Save changes",
    source_coded: "Save changes",
    source_spans: [],
    targets: { "fr-FR": { text: "Enregistrer les modifications", status: "translated" } },
    translatable: true,
    has_spans: false,
    properties: {},
  },
  {
    id: "b3",
    source: "Delete account",
    source_coded: "Delete account",
    source_spans: [],
    targets: {
      "fr-FR": { text: "Supprimer le compte", status: "reviewed" },
      "de-DE": { text: "Konto löschen", status: "translated" },
    },
    translatable: true,
    has_spans: false,
    properties: {},
  },
];

const localeStat = (over: Record<string, unknown>) => ({
  locale: "fr-FR",
  translated_blocks: 2,
  total_blocks: 3,
  translated_words: 0,
  total_words: 0,
  percentage: 66,
  approved_blocks: 0,
  failing_checks: 0,
  ship_state: "pending" as const,
  compliance_rate: 0.82,
  compliance_basis: "voice+checks" as const,
  compliant_blocks: 2,
  ...over,
});

const pendingStats: TranslationDashboardStats = {
  locale_stats: [
    localeStat({}),
    localeStat({ locale: "de-DE", translated_blocks: 1, approved_blocks: 0 }),
  ],
  item_stats: [
    {
      item_name: "messages.json",
      item_id: "itm-msg1",
      format: "json",
      collection_id: "coll-default",
      block_count: 3,
      word_count: 0,
      locales: [
        localeStat({}),
        localeStat({ locale: "de-DE", translated_blocks: 1, approved_blocks: 0 }),
      ],
    },
  ],
  collection_stats: [],
  total_blocks: 3,
  translatable_blocks: 3,
  total_source_words: 0,
};

const clearStats: TranslationDashboardStats = {
  locale_stats: [localeStat({ translated_blocks: 2, approved_blocks: 2, ship_state: "governed" })],
  item_stats: [
    {
      item_name: "messages.json",
      item_id: "itm-msg1",
      format: "json",
      collection_id: "coll-default",
      block_count: 3,
      word_count: 0,
      locales: [localeStat({ translated_blocks: 2, approved_blocks: 2, ship_state: "governed" })],
    },
  ],
  collection_stats: [],
  total_blocks: 3,
  translatable_blocks: 3,
  total_source_words: 0,
};

/**
 * The terminology and voice evidence the server stamps on a queue entry, and
 * the same map the mock's review context reads. `b1` sits under the cursor and
 * is below its profile's bar; `b2` clears every bar the server applies, so the
 * queue shows both verdicts and each agrees with the context beside it.
 */
const blockEvidence = {
  b1: { term_compliance: "compliant" as const, voice_score: 62, voice_bar: 90 },
  b2: { term_compliance: "compliant" as const, voice_score: 94, voice_bar: 90 },
};

/** Give the flex-1/min-h-0 session a bounded height in Storybook. */
const Frame = (Story: () => ReactNode) => (
  <div className="flex h-[640px] flex-col overflow-hidden rounded-lg border border-border bg-background">
    {Story()}
  </div>
);

const meta: Meta<typeof ReviewSession> = {
  title: "Review/ReviewSession",
  component: ReviewSession,
  parameters: { layout: "fullscreen" },
  decorators: [Frame, createProvidersDecorator(blocks, { blockEvidence })],
};
export default meta;
type Story = StoryObj<typeof ReviewSession>;

/**
 * The queue + focused reviewer, with the "Approve all passing" fast path and
 * the five layers of context the server resolved for the unit under the cursor:
 * the point rail, the neighbours, the anchored findings, the content-memory
 * match, and the provenance.
 */
export const Default: Story = {
  args: { project: sampleProject, dashboardStats: pendingStats, stream: "main" },
};

/** All-clear: nothing pending review. */
export const AllClear: Story = {
  args: { project: sampleProject, dashboardStats: clearStats, stream: "main" },
};

/**
 * The same queue for a unit nothing governs: no profile bound, no terms
 * matched, no neighbours, no content-memory match and no decision recorded.
 * The layers each name their own emptiness, so an ungoverned project reads as
 * ungoverned rather than as a surface that failed to load.
 */
export const NothingResolved: Story = {
  args: { project: sampleProject, dashboardStats: pendingStats, stream: "main" },
  decorators: [
    createProvidersDecorator(blocks, {
      getReviewContext: async (_ws, _projectId, itemName, blockId, targetLocale) =>
        emptyReviewContext(blockId, itemName, targetLocale),
    }),
  ],
};
