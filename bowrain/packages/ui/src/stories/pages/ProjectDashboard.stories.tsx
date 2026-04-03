import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ProjectDashboard } from "../../components/ProjectDashboard";
import { withProviders } from "../decorators";
import { sampleProject } from "../fixtures";
import type { ProjectInfo } from "../../types/api";

const meta: Meta<typeof ProjectDashboard> = {
  title: "Pages/Workspace/ProjectDashboard",
  component: ProjectDashboard,
  tags: ["autodocs"],
  decorators: [
    withProviders,
    (Story) => (
      <div style={{ maxWidth: 960, padding: 24 }}>
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
// Stories
// ---------------------------------------------------------------------------

/** Empty workspace — shows the onboarding experience with getting-started pathways. */
export const Empty: Story = {
  args: {
    projects: [],
    onCreateProject: fn(),
    onOpenProject: fn(),
    onCreateSampleProject: fn(),
    workspaceName: "My Workspace",
  },
};

/** Empty workspace without sample project option. */
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

/** Multiple projects with varied sizes and language counts. */
export const WithProjects: Story = {
  args: {
    projects: [sampleProject, marketingProject, mobileProject, docsProject, wpProject],
    onCreateProject: fn(),
    onOpenProject: fn(),
    workspaceName: "Acme Corp",
  },
};

/** Many projects to demonstrate grid scaling. */
export const ManyProjects: Story = {
  args: {
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
