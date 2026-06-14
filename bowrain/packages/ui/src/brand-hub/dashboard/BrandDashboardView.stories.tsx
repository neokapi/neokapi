import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import type { Decorator } from "@storybook/react";
import { BrandDashboardView } from "./BrandDashboardView";
import { brandHubOverrides } from "../../stories/brandHubFixtures";
import { createProvidersDecorator } from "../../stories/decorators";
import type { ApiAdapter } from "../../api/adapter";

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
  { dimension: "brand_compliance", score: 81, penalty: 8, issues: 2 },
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

const complianceOverrides: Partial<ApiAdapter> = {
  listProjects: async () => projects as never,
  getBrandScores: async () => storedScores as never,
  getBrandTrends: async () => trends as never,
  getBrandDrift: async () => ({
    drifted: true,
    recent_avg: 76.4,
    baseline_avg: 83.1,
    drop: 6.7,
    recent_days: 7,
    recent_count: 14,
    reason: "vocabulary slips on the new landing pages",
  }),
  listBrandProfiles: async () => profiles as never,
};

const populated: Decorator = createProvidersDecorator(undefined, {
  ...brandHubOverrides,
  ...complianceOverrides,
});

const emptyOverrides: Partial<ApiAdapter> = {
  ...brandHubOverrides,
  listConcepts: async () => ({ concepts: [], total_count: 0 }),
  listChangesets: async () => [],
  listMarkets: async () => [],
  listProjects: async () => [],
  listBrandProfiles: async () => [],
};

const empty: Decorator = createProvidersDecorator(undefined, emptyOverrides);

const meta: Meta<typeof BrandDashboardView> = {
  title: "Brand Hub/Dashboard/BrandDashboardView",
  component: BrandDashboardView,
  tags: ["autodocs"],
  parameters: { layout: "fullscreen" },
  args: {
    onOpenExperiment: fn(),
    onViewExperiments: fn(),
    onViewConcepts: fn(),
    onViewVoice: fn(),
    onOpenConcept: fn(),
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
type Story = StoryObj<typeof BrandDashboardView>;

export const Default: Story = {
  decorators: [populated],
};

export const Empty: Story = {
  decorators: [empty],
};
