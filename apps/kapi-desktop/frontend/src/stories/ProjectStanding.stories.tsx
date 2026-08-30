import type { Meta, StoryObj } from "@storybook/react-vite";
import { ProjectStanding, type ProjectPointsResult } from "../components/ProjectStanding";
import type { KapiProject, ProjectStatus } from "../types/api";

const project: KapiProject = {
  version: "v1",
  name: "Northsea",
  defaults: { source_language: "en-US", coordinates: { brand: "northsea" } },
};

const status: ProjectStatus = {
  projectPath: "/p/northsea/kapi.yaml",
  projectName: "Northsea",
  hasData: true,
  collections: [
    { name: "Docs", blockCount: 412, coverage: {}, targetLanguages: [] },
    { name: "App", blockCount: 96, coverage: {}, targetLanguages: [] },
  ],
};

const manyPoints: ProjectPointsResult = {
  at: "2026-08-30T09:00:00Z",
  points: [
    {
      ref: "",
      label: "project default",
      default: true,
      coordinates: { brand: "northsea" },
      collections: ["App"],
      voice: "Northsea",
      voice_field: "defaults.voice",
      termstore: ".kapi/terms.json",
    },
    {
      ref: "support/docs",
      label: "support/docs",
      profile: "support",
      channel: "docs",
      default: false,
      coordinates: { brand: "northsea", product: "support", channel: "docs" },
      collections: ["Docs"],
      voice: "Northsea Support",
      voice_field: "profiles.support.voice",
    },
    {
      ref: "campaign/promo",
      label: "campaign/promo",
      profile: "campaign",
      channel: "promo",
      default: false,
      coordinates: { brand: "northsea", product: "campaign", channel: "promo" },
      collections: [],
      voice: "Northsea",
      fallback: {
        profile: "campaign",
        expired: true,
        boundary: "2026-08-29T00:00:00Z",
        message: 'profile "campaign" expired 2026-08-29',
      },
    },
  ],
};

const meta: Meta<typeof ProjectStanding> = {
  title: "Pages/Project Standing",
  component: ProjectStanding,
  parameters: { layout: "padded" },
  args: { tabID: "t1", project, displayName: "Northsea", status },
};

export default meta;
type Story = StoryObj<typeof ProjectStanding>;

/** Several points, one of them governed by a profile whose window closed. */
export const ManyPoints: Story = {
  args: {
    points: manyPoints,
    server: { connected: true, host: "app.bowrain.cloud", stream: "main" },
  },
};

/** The common shape: one point, one quiet row, and nothing missing. */
export const SinglePoint: Story = {
  args: {
    points: { at: manyPoints.at, points: [manyPoints.points[0]] },
    server: { connected: false, stream: "main" },
  },
};

/** A project that declares targets gains the languages axis. */
export const WithTargetLanguages: Story = {
  args: {
    project: {
      ...project,
      defaults: { ...project.defaults, target_languages: ["nb-NO", "de-DE"] },
    },
    points: manyPoints,
    server: { connected: false, stream: "main" },
  },
};

/** Nothing extracted yet: the counts say so rather than reading as zero. */
export const NotExtracted: Story = {
  args: {
    status: { ...status, hasData: false },
    points: { at: manyPoints.at, points: [manyPoints.points[0]] },
    server: { connected: false, stream: "main" },
  },
};
