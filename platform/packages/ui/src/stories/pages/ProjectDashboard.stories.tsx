import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ProjectDashboard } from "../../components/ProjectDashboard";
import { withProviders } from "../decorators";
import { sampleProject } from "../fixtures";

const meta: Meta<typeof ProjectDashboard> = {
  title: "Pages/ProjectDashboard",
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

export const WithProjects: Story = {
  args: {
    projects: [
      sampleProject,
      {
        ...sampleProject,
        id: "proj-2",
        name: "Marketing Website",
        source_locale: "en-US",
        target_locales: ["fr-FR", "de-DE", "ja-JP", "es-ES", "zh-CN"],
        items: [
          {
            name: "landing.html",
            format: "html",
            type: "file",
            size: 12000,
            block_count: 48,
            word_count: 320,
          },
          {
            name: "about.html",
            format: "html",
            type: "file",
            size: 8000,
            block_count: 32,
            word_count: 210,
          },
        ],
        created_at: "2025-12-01T10:00:00Z",
        modified_at: "2026-02-18T09:15:00Z",
      },
      {
        ...sampleProject,
        id: "proj-3",
        name: "Mobile App Strings",
        source_locale: "en",
        target_locales: ["fr", "de"],
        items: [
          {
            name: "strings.xml",
            format: "xml",
            type: "file",
            size: 3200,
            block_count: 120,
            word_count: 650,
          },
        ],
        created_at: "2026-01-15T10:00:00Z",
        modified_at: "2026-02-25T16:45:00Z",
      },
    ],
    onCreateProject: fn(),
    onOpenProject: fn(),
  },
};

export const Empty: Story = {
  args: {
    projects: [],
    onCreateProject: fn(),
    onOpenProject: fn(),
  },
};
