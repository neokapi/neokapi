import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import type { Decorator } from "@storybook/react";
import { VoiceDashboardView } from "./VoiceDashboardView";
import { voiceHubOverrides } from "../../stories/voiceHubFixtures";
import { createProvidersDecorator } from "../../stories/decorators";
import type { ApiAdapter } from "../../api/adapter";
import type { CandidateRule } from "../../voice/types";

// Compliance is project-scoped and not part of the shared brand fixtures, so the
// stories layer a couple of projects with brand-check history on top.
const projects = [
  { id: "p-web", name: "Marketing Website" },
  { id: "p-app", name: "Mobile App" },
];

const dimensions = [
  { dimension: "tone", score: 88, penalty: 4, issues: 1 },
  { dimension: "style", score: 79, penalty: 9, issues: 3 },
  { dimension: "vocabulary", score: 72, penalty: 14, issues: 5 },
  { dimension: "clarity", score: 84, penalty: 6, issues: 2 },
  { dimension: "compliance", score: 81, penalty: 8, issues: 2 },
];

const storedScores = Array.from({ length: 14 }).map((_, i) => ({
  id: `s-${i}`,
  project_id: "p-web",
  stream: "main",
  block_id: `block-${i}`,
  profile_id: "vp-1",
  locale: i % 2 === 0 ? "en-US" : "de-DE",
  score: 70 + ((i * 7) % 28),
  dimensions,
  findings: [],
  checked_at: `2026-06-${String(13 - (i % 12)).padStart(2, "0")}T10:00:00Z`,
}));

const trends = Array.from({ length: 8 }).map((_, i) => ({
  date: `2026-06-${String(6 + i).padStart(2, "0")}`,
  avg_score: 74 + Math.round(Math.sin(i / 1.6) * 6) + i,
  count: 8 + i,
}));

const profiles = [
  { id: "vp-1", name: "Core voice" },
  { id: "vp-2", name: "Support voice" },
];

const rollup = {
  projects: [
    {
      project_id: "p-web",
      project_name: "Marketing Website",
      profile_id: "vp-1",
      profile_name: "Core voice",
      overall: 78,
      dimensions,
      trend: "down" as const,
      drift: {
        drifted: true,
        recent_avg: 76.4,
        baseline_avg: 83.1,
        drop: 6.7,
        recent_days: 7,
        recent_count: 14,
        reason: "vocabulary slips on the new landing pages",
      },
      scored_blocks: 14,
      last_scored_at: "2026-06-13T10:00:00Z",
    },
    {
      project_id: "p-app",
      project_name: "Mobile App",
      profile_id: "vp-2",
      profile_name: "Support voice",
      overall: 91,
      dimensions,
      trend: "up" as const,
      scored_blocks: 6,
      last_scored_at: "2026-06-12T09:00:00Z",
    },
    {
      project_id: "p-docs",
      project_name: "Docs Portal",
      overall: null,
      trend: "" as const,
      scored_blocks: 0,
      last_scored_at: null,
    },
  ],
  total: 3,
  limit: 50,
  offset: 0,
};

// What each profile's review lists: a phrasing the team corrected two ways and
// two suggestions on the core voice, one approved rule on the support voice.
const candidatesByProfile: Record<string, CandidateRule[]> = {
  "vp-1": [
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
    {
      term: "utilize",
      replacement: "use",
      correction_count: 7,
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
  ],
  "vp-2": [
    {
      term: "Globex",
      replacement: "",
      correction_count: 5,
      dimension: "vocabulary",
      status: "approved",
    },
    {
      term: "synergy",
      replacement: "teamwork",
      correction_count: 4,
      dimension: "vocabulary",
      status: "pending",
    },
  ],
};

const complianceOverrides: Partial<ApiAdapter> = {
  listVoiceCandidates: async (_ws, profileId) => candidatesByProfile[profileId] ?? [],
  listProjects: async () => projects as never,
  getVoiceScores: async () => storedScores as never,
  getVoiceTrends: async () => trends as never,
  getVoiceDrift: async () => ({
    drifted: true,
    recent_avg: 76.4,
    baseline_avg: 83.1,
    drop: 6.7,
    recent_days: 7,
    recent_count: 14,
    reason: "vocabulary slips on the new landing pages",
  }),
  getVoiceRollup: async () => rollup as never,
  listVoiceProfiles: async () => profiles as never,
};

const populated: Decorator = createProvidersDecorator(undefined, {
  ...voiceHubOverrides,
  ...complianceOverrides,
});

// An empty workspace has to answer the aggregates as emptily as the lists — the
// dashboard's metric cards read the counts, not the list lengths.
const emptyOverrides: Partial<ApiAdapter> = {
  ...voiceHubOverrides,
  listConcepts: async () => ({ concepts: [], total_count: 0 }),
  listChangesets: async () => [],
  listMarkets: async () => [],
  listProjects: async () => [],
  listVoiceProfiles: async () => [],
  getConceptStatusCounts: async () => ({ total: 0, by_status: {} }),
  getConceptLocaleCoverage: async () => ({ total: 0, locales: [] }),
  getChangesetCounts: async () => ({ total: 0, by_status: {} }),
};

const empty: Decorator = createProvidersDecorator(undefined, emptyOverrides);

const meta: Meta<typeof VoiceDashboardView> = {
  title: "Context/Dashboard/VoiceDashboardView",
  component: VoiceDashboardView,
  tags: ["autodocs"],
  parameters: { layout: "fullscreen" },
  args: {
    onOpenExperiment: fn(),
    onViewExperiments: fn(),
    onViewConcepts: fn(),
    onViewVoice: fn(),
    onOpenConcept: fn(),
    onOpenProject: fn(),
    onOpenReview: fn(),
    onViewActivity: fn(),
    // Stories on one docs page would all answer the same key.
    keyboard: false,
  },
  decorators: [
    (Story) => (
      <div style={{ padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof VoiceDashboardView>;

/** Counts, What's new with change-sets and suggested rules, the projects, then the rest. */
export const Default: Story = {
  decorators: [populated],
  globals: { theme: "light" },
};

export const DefaultDark: Story = {
  decorators: [populated],
  globals: { theme: "dark" },
};

/** Nothing waits: no change-set in review and no suggested rule. */
export const NothingWaits: Story = {
  decorators: [
    createProvidersDecorator(undefined, {
      ...voiceHubOverrides,
      ...complianceOverrides,
      listChangesets: async () => [],
      getChangesetCounts: async () => ({ total: 0, by_status: {} }),
      listVoiceCandidates: async () => [],
    }),
  ],
  globals: { theme: "light" },
};

/** Long project, profile and change-set names truncate on their rows. */
export const LongContent: Story = {
  decorators: [
    createProvidersDecorator(undefined, {
      ...voiceHubOverrides,
      ...complianceOverrides,
      getVoiceRollup: async () =>
        ({
          ...rollup,
          projects: rollup.projects.map((p) => ({
            ...p,
            project_name: `${p.project_name} for the northern European storefronts and their help centre`,
            profile_name: p.profile_name && `${p.profile_name} for customer support and sales`,
          })),
        }) as never,
      listVoiceProfiles: async () =>
        profiles.map((p) => ({
          ...p,
          name: `${p.name} for every customer-facing channel`,
        })) as never,
    }),
  ],
  globals: { theme: "light" },
};

/** A workspace with nothing in it yet. */
export const Empty: Story = {
  decorators: [empty],
  globals: { theme: "light" },
};
