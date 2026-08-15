import type { Meta, StoryObj } from "@storybook/react-vite";
import type { PostHogConnectorConfig, PostHogDemandResponse } from "@neokapi/ui";
import { LocaleDemandPage, type LocaleDemandApi } from "./LocaleDemandPage";

// Locale demand page with the PostHog source wired to a mocked fetch layer —
// the live-provenance variant used for the PR screenshots, plus the
// unconnected fallback (sample dataset under its sample-data label).

/** Deterministic 12-week trend around a weekly base. */
function trend(base: number, growth: number) {
  return Array.from({ length: 12 }, (_, week) => ({
    week_start: `2026-0${1 + Math.floor(week / 5)}-0${1 + (week % 5)}`,
    sessions: Math.round(base * Math.pow(1 + growth, week - 11)),
  }));
}

const wireSnapshot: PostHogDemandResponse = {
  range: "30d",
  generated_at: "2026-07-03T08:41:00Z",
  total_sessions: 184_320,
  countries: [
    {
      country_code: "US",
      sessions: 61_240,
      languages: [
        { tag: "en-US", sessions: 52_100 },
        { tag: "es-419", sessions: 6_890 },
        { tag: "zh-Hans", sessions: 2_250 },
      ],
    },
    {
      country_code: "BR",
      sessions: 38_180,
      languages: [
        { tag: "pt-BR", sessions: 35_640 },
        { tag: "en-US", sessions: 2_540 },
      ],
    },
    {
      country_code: "DE",
      sessions: 27_930,
      languages: [
        { tag: "de-DE", sessions: 23_120 },
        { tag: "en-US", sessions: 3_410 },
        { tag: "tr-TR", sessions: 1_400 },
      ],
    },
    {
      country_code: "JP",
      sessions: 24_400,
      languages: [
        { tag: "ja-JP", sessions: 23_470 },
        { tag: "en-US", sessions: 930 },
      ],
    },
    {
      country_code: "KR",
      sessions: 18_890,
      languages: [
        { tag: "ko-KR", sessions: 17_950 },
        { tag: "en-US", sessions: 940 },
      ],
    },
    {
      country_code: "MX",
      sessions: 13_680,
      languages: [
        { tag: "es-419", sessions: 12_720 },
        { tag: "en-US", sessions: 960 },
      ],
    },
  ],
  languages: [
    { tag: "en-US", sessions: 60_880, trend: trend(14_200, 0.004) },
    { tag: "pt-BR", sessions: 35_640, trend: trend(8_100, 0.055) },
    { tag: "de-DE", sessions: 23_120, trend: trend(5_400, 0.008) },
    { tag: "ja-JP", sessions: 23_470, trend: trend(5_500, 0.021) },
    { tag: "es-419", sessions: 19_610, trend: trend(4_500, 0.037) },
    { tag: "ko-KR", sessions: 17_950, trend: trend(4_100, 0.048) },
    // A long-tail language the trend query had no rows for: renders as
    // "no data", not as zero demand.
    { tag: "tr-TR", sessions: 1_400, trend: [] },
  ],
  served_locale_hit_rate: 0.71,
  source: { kind: "posthog", host_label: "eu.posthog.com", posthog_project_id: "12345" },
  cached_at: "2026-07-03T09:12:00Z",
  cached: true,
};

const savedConfig: PostHogConnectorConfig = {
  configured: true,
  host: "eu",
  host_label: "eu.posthog.com",
  posthog_project_id: "12345",
  api_key_masked: "••••9fk2",
  updated_at: "2026-07-01T09:12:00Z",
};

const liveApi: LocaleDemandApi = {
  getPostHogConnector: async () => savedConfig,
  savePostHogConnector: async () => savedConfig,
  getPostHogDemand: async (_ws, _pid, range) => ({ ...wireSnapshot, range }),
};

const unconnectedApi: LocaleDemandApi = {
  getPostHogConnector: async () => ({ configured: false }),
  savePostHogConnector: async () => savedConfig,
  getPostHogDemand: async () => {
    throw new Error(
      `404: ${JSON.stringify({ error: "PostHog connector is not configured", code: "not_configured" })}`,
    );
  },
};

const meta: Meta<typeof LocaleDemandPage> = {
  title: "Locale Demand/Page (PostHog wired)",
  component: LocaleDemandPage,
  parameters: { layout: "fullscreen" },
  args: {
    workspaceSlug: "acme",
    projectId: "proj-1",
    projectLocales: ["en", "de", "ja"],
  },
};

export default meta;
type Story = StoryObj<typeof LocaleDemandPage>;

/** Connected: live PostHog data, live badge, provenance footer with host · project · cache time. */
export const LiveProvenance: Story = {
  args: { api: liveApi },
};

/** Unconnected: sample dataset under the sample-data notice, with the connect button. */
export const Unconnected: Story = {
  args: { api: unconnectedApi },
};
