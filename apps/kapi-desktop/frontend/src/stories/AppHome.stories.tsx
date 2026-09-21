import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { AppHome } from "../components/AppHome";
import type { WorkspaceHome } from "../types/api";

const LOCATION = "/fakehome/.local/share/kapi/workspaces/default";

function workspace(projects: WorkspaceHome["projects"]): WorkspaceHome {
  return { location: LOCATION, read_only: false, projects };
}

const meta: Meta<typeof AppHome> = {
  title: "Components/AppHome",
  component: AppHome,
  tags: ["autodocs"],
  args: {
    samplesDismissed: false,
    onOpenCheckout: fn(),
    onOpenContext: fn(),
    onForgetProject: fn(),
    onNewProject: fn(),
    onOpenProject: fn(),
    onNavigate: fn(),
    onCreateSampleProject: fn(),
    onDismissSamples: fn(),
  },
  parameters: {
    layout: "fullscreen",
  },
};

export default meta;
type Story = StoryObj<typeof AppHome>;

/** The ordinary case: a few projects, each with one checkout here. */
export const WithProjects: Story = {
  args: {
    workspace: workspace([
      {
        key: "prj_acmeapp",
        name: "Acme App",
        last_active: "2026-09-20T14:30:00Z",
        checkouts: [
          {
            path: "/fakehome/projects/acme-app",
            recipe: "/fakehome/projects/acme-app/kapi.yaml",
            missing: false,
          },
        ],
      },
      {
        key: "prj_website",
        name: "Website",
        last_active: "2026-09-18T09:15:00Z",
        checkouts: [
          {
            path: "/fakehome/projects/website",
            recipe: "/fakehome/projects/website/kapi.yaml",
            missing: false,
          },
        ],
      },
    ]),
  },
};

/** Nothing has registered yet, so the sample cards carry the screen. */
export const Empty: Story = {
  args: { workspace: workspace([]) },
};

/** Sample cards hidden after the user dismisses them. */
export const SamplesDismissed: Story = {
  args: { workspace: workspace([]), samplesDismissed: true },
};

/**
 * Two worktrees of one repository are one project with two places to open it
 * from; a checkout that went away is shown as missing rather than dropped; and
 * a project only ever opened on another machine still opens, on its context.
 */
export const CheckoutsAndLosses: Story = {
  args: {
    samplesDismissed: true,
    workspace: workspace([
      {
        key: "prj_kapimart",
        name: "KapiMart",
        last_active: "2026-09-21T08:05:00Z",
        checkouts: [
          {
            path: "/fakehome/src/kapimart",
            recipe: "/fakehome/src/kapimart/kapi.yaml",
            missing: false,
          },
          {
            path: "/fakehome/src/kapimart-release",
            recipe: "/fakehome/src/kapimart-release/kapi.yaml",
            missing: false,
          },
        ],
      },
      {
        key: "prj_movedaway",
        name: "Old Handbook",
        last_active: "2026-08-02T11:00:00Z",
        checkouts: [
          {
            path: "/fakehome/projects/handbook",
            recipe: "/fakehome/projects/handbook/kapi.yaml",
            missing: true,
          },
        ],
      },
      {
        key: "prj_elsewhere",
        name: "Field Guide",
        last_active: "2026-09-10T16:45:00Z",
        checkouts: [],
      },
    ]),
  },
};

/** A workspace directory that refuses writes: readable, and nothing new joins. */
export const ReadOnlyWorkspace: Story = {
  args: {
    samplesDismissed: true,
    workspace: {
      location: LOCATION,
      read_only: true,
      projects: [
        {
          key: "prj_acmeapp",
          name: "Acme App",
          last_active: "2026-09-20T14:30:00Z",
          checkouts: [
            {
              path: "/fakehome/projects/acme-app",
              recipe: "/fakehome/projects/acme-app/kapi.yaml",
              missing: false,
            },
          ],
        },
      ],
    },
  },
};

/** The workspace could not be opened at all. */
export const WorkspaceUnreadable: Story = {
  args: {
    samplesDismissed: true,
    workspace: null,
    workspaceError: new Error("workspace: /fakehome/Dropbox/kapi is inside a synchronized folder"),
  },
};
