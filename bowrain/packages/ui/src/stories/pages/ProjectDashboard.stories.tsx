import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ProjectDashboard } from "../../components/ProjectDashboard";
import type { LoopStatusData } from "../../components/LoopStatusRow";
import { withProviders } from "../decorators";
import { sampleProject } from "../fixtures";
import type { ProjectInfo } from "../../types/api";

const meta: Meta<typeof ProjectDashboard> = {
  title: "Pages/Workspace/ProjectDashboard",
  component: ProjectDashboard,
  tags: ["autodocs"],
  decorators: [
    withProviders,
    // The dashboard brings its own max-w-6xl container; give it a full-width
    // stage so the first-run hero and loop-status row lay out as in the app.
    (Story) => (
      <div style={{ padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ProjectDashboard>;

// ---------------------------------------------------------------------------
// Fixtures for richer stories
// ---------------------------------------------------------------------------

const marketingProject: ProjectInfo = {
  ...sampleProject,
  id: "proj-2",
  name: "Marketing Website",
  default_source_language: "en-US",
  target_languages: ["fr-FR", "de-DE", "ja-JP", "es-ES", "zh-CN"],
  items: [
    {
      id: "itm-land",
      name: "landing.html",
      format: "html",
      type: "file",
      size: 12000,
      block_count: 48,
      word_count: 1320,
    },
    {
      id: "itm-abt",
      name: "about.html",
      format: "html",
      type: "file",
      size: 8000,
      block_count: 32,
      word_count: 910,
    },
    {
      id: "itm-prc",
      name: "pricing.json",
      format: "json",
      type: "file",
      size: 3000,
      block_count: 20,
      word_count: 240,
    },
  ],
  streams: [
    {
      name: "main",
      parent: "",
      base_cursor: 0,
      archived: false,
      visibility: "public",
      description: "",
      created_at: "2025-12-01T10:00:00Z",
      created_by: "user-1",
    },
    {
      name: "q1-campaign",
      parent: "main",
      base_cursor: 5,
      archived: false,
      visibility: "public",
      description: "Q1 marketing campaign",
      created_at: "2026-02-01T10:00:00Z",
      created_by: "user-1",
    },
  ],
  created_at: "2025-12-01T10:00:00Z",
  modified_at: "2026-03-13T09:15:00Z",
};

const mobileProject: ProjectInfo = {
  ...sampleProject,
  id: "proj-3",
  name: "Mobile App Strings",
  default_source_language: "en",
  target_languages: ["fr", "de"],
  items: [
    {
      id: "itm-str",
      name: "strings.xml",
      format: "xml",
      type: "file",
      size: 3200,
      block_count: 120,
      word_count: 650,
    },
  ],
  created_at: "2026-01-15T10:00:00Z",
  modified_at: "2026-03-12T16:45:00Z",
};

const docsProject: ProjectInfo = {
  ...sampleProject,
  id: "proj-4",
  name: "API Documentation",
  default_source_language: "en",
  target_languages: ["ja", "ko", "zh-CN", "pt-BR"],
  items: [
    {
      id: "itm-gs",
      name: "getting-started.md",
      format: "md",
      type: "file",
      size: 15000,
      block_count: 85,
      word_count: 2400,
    },
    {
      id: "itm-api",
      name: "api-reference.md",
      format: "md",
      type: "file",
      size: 28000,
      block_count: 200,
      word_count: 5800,
    },
    {
      id: "itm-cl",
      name: "changelog.md",
      format: "md",
      type: "file",
      size: 4000,
      block_count: 30,
      word_count: 450,
    },
  ],
  created_at: "2025-10-20T10:00:00Z",
  modified_at: "2026-03-14T11:30:00Z",
};

const wpProject: ProjectInfo = {
  ...sampleProject,
  id: "proj-5",
  name: "WordPress Blog",
  default_source_language: "en-US",
  target_languages: ["es-ES", "fr-FR", "pt-BR"],
  items: [
    {
      id: "itm-post",
      name: "posts.xliff",
      format: "xliff",
      type: "file",
      size: 45000,
      block_count: 310,
      word_count: 8200,
    },
    {
      id: "itm-page",
      name: "pages.xliff",
      format: "xliff",
      type: "file",
      size: 12000,
      block_count: 60,
      word_count: 1500,
    },
  ],
  created_at: "2026-02-01T10:00:00Z",
  modified_at: "2026-03-10T08:20:00Z",
};

// ---------------------------------------------------------------------------
// Loop-status fixtures
// ---------------------------------------------------------------------------

const loopStatus: LoopStatusData = {
  latestActivity: {
    summary: "Translate flow completed for Marketing Website (fr-FR)",
    created_at: new Date(Date.now() - 42 * 60 * 1000).toISOString(),
  },
  latestRun: {
    state: "parked",
    stallReason: "needs_credits",
    projectName: "Marketing Website",
    updatedAt: new Date(Date.now() - 55 * 60 * 1000).toISOString(),
  },
  openReviewTasks: 7,
  ship: { governed: 3, aiShippable: 2, pending: 5, countedProjects: 2, totalProjects: 2 },
  brand: { averageScore: 86, scoredProjects: 3, driftingProjects: 1 },
};

const loopStatusAllClear: LoopStatusData = {
  latestActivity: undefined,
  openReviewTasks: 0,
  brand: { averageScore: null, scoredProjects: 0, driftingProjects: 0 },
};

// ---------------------------------------------------------------------------
// Stories
// ---------------------------------------------------------------------------

/**
 * First run (0 projects): the assistant-driven starter-pack hero with the
 * copyable prompt, the create/import card with its drop affordance, and the
 * invite-team card. Hosted brand scan is offered as the no-agent fallback.
 */
export const Empty: Story = {
  args: {
    projects: [],
    onCreateProject: fn(),
    onOpenProject: fn(),
    onCreateSampleProject: fn(),
    onScanBrand: fn(),
    onInviteTeam: fn(),
    workspaceName: "My Workspace",
    serverUrl: "https://app.bowrain.cloud",
  },
};

/** First run without sample project, brand scan, or invite wiring (minimal shell). */
export const EmptyNoSample: Story = {
  args: {
    projects: [],
    onCreateProject: fn(),
    onOpenProject: fn(),
    workspaceName: "Acme Corp",
  },
};

/** Single project — minimal dashboard. */
export const SingleProject: Story = {
  args: {
    projects: [sampleProject],
    onCreateProject: fn(),
    onOpenProject: fn(),
    workspaceName: "My Workspace",
  },
};

/** Multiple projects with the loop-status layer above the grid. */
export const WithProjects: Story = {
  args: {
    projects: [sampleProject, marketingProject, mobileProject, docsProject, wpProject],
    onCreateProject: fn(),
    onOpenProject: fn(),
    workspaceName: "Acme Corp",
    loopStatus,
    onOpenActivities: fn(),
    onOpenRuns: fn(),
    onOpenTasks: fn(),
    onOpenDelivery: fn(),
    onOpenBrandDashboard: fn(),
  },
};

/** Populated workspace whose loop is idle: no activity, no reviews, no scores. */
export const LoopAllClear: Story = {
  args: {
    projects: [sampleProject, mobileProject],
    onCreateProject: fn(),
    onOpenProject: fn(),
    workspaceName: "Acme Corp",
    loopStatus: loopStatusAllClear,
    onOpenActivities: fn(),
    onOpenTasks: fn(),
    onOpenBrandDashboard: fn(),
  },
};

/** Many projects to demonstrate grid scaling under the loop-status layer. */
export const ManyProjects: Story = {
  args: {
    loopStatus,
    onOpenActivities: fn(),
    onOpenTasks: fn(),
    onOpenBrandDashboard: fn(),
    projects: [
      sampleProject,
      marketingProject,
      mobileProject,
      docsProject,
      wpProject,
      {
        ...sampleProject,
        id: "proj-6",
        name: "iOS Storyboard",
        target_languages: ["ja", "ko"],
        modified_at: "2026-03-08T14:00:00Z",
      },
      {
        ...mobileProject,
        id: "proj-7",
        name: "Android Resources",
        modified_at: "2026-03-05T10:00:00Z",
      },
      {
        ...docsProject,
        id: "proj-8",
        name: "Help Center",
        target_languages: ["fr", "de", "es"],
        modified_at: "2026-03-01T08:00:00Z",
      },
    ],
    onCreateProject: fn(),
    onOpenProject: fn(),
    workspaceName: "Globalize Inc",
  },
};
