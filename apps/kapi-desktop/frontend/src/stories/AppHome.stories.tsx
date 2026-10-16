import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { AppHome } from "../components/AppHome";
import { documentConflict } from "./fixtures/keptConflicts";
import {
  LOCATION,
  ONE_PROJECT_ROWS,
  SEVERAL_ROWS,
  WORKSPACE_RULES,
  workspaceOf,
} from "./fixtures/standings";
import type { WorkspaceHome } from "../types/api";

function workspace(projects: WorkspaceHome["projects"]): WorkspaceHome {
  return { location: LOCATION, read_only: false, projects };
}

const meta: Meta<typeof AppHome> = {
  title: "Components/AppHome",
  component: AppHome,
  tags: ["autodocs"],
  args: {
    workspaceRules: [],
    startHereDismissed: false,
    hasOpenProject: false,
    onDismissStartHere: fn(),
    onConnectAssistant: fn(),
    onOpenWhatsNew: fn(),
    onOpenCheckout: fn(),
    onOpenContext: fn(),
    onForgetProject: fn(),
    onNewProject: fn(),
    onOpenProject: fn(),
    onNavigate: fn(),
    onCreateSampleProject: fn(),
  },
  parameters: {
    layout: "fullscreen",
  },
};

export default meta;
type Story = StoryObj<typeof AppHome>;

const dark = { theme: "dark" };

/**
 * The first run: nothing has registered, nothing is recorded, and Start here
 * carries the screen. The counts wait until there is a project to count.
 */
export const FirstRun: Story = {
  args: { workspace: workspace([]) },
};
export const FirstRunDark: Story = { args: FirstRun.args, globals: dark };

/**
 * One project with four new items while claude-code is working in it: the row
 * carries claude-code's mark and a working chip, and Start here comes last.
 */
export const OneProjectWorking: Story = {
  args: {
    workspace: workspaceOf(ONE_PROJECT_ROWS),
    hasOpenProject: true,
  },
};
export const OneProjectWorkingDark: Story = { args: OneProjectWorking.args, globals: dark };

/**
 * Several projects, newest session first. KapiMart holds a disagreement, so
 * its row carries a "1 needs you" chip and reads Decide, and "needs you" is
 * the one count in the warning ink. Field Guide is quiet and has no row under
 * What's new.
 */
export const SeveralProjects: Story = {
  args: {
    workspace: workspaceOf(SEVERAL_ROWS),
    startHereDismissed: true,
  },
};
export const SeveralProjectsDark: Story = { args: SeveralProjects.args, globals: dark };

/** Two rules hold in every project: one kept in a review, one merged. */
export const WithWorkspaceRules: Story = {
  args: {
    workspace: workspaceOf(SEVERAL_ROWS),
    workspaceRules: WORKSPACE_RULES,
    startHereDismissed: true,
  },
};
export const WithWorkspaceRulesDark: Story = { args: WithWorkspaceRules.args, globals: dark };

/** A workspace directory that refuses writes: readable, and nothing new joins. */
export const ReadOnlyWorkspace: Story = {
  args: {
    startHereDismissed: true,
    workspace: workspaceOf(ONE_PROJECT_ROWS, true),
  },
};
export const ReadOnlyWorkspaceDark: Story = { args: ReadOnlyWorkspace.args, globals: dark };

/**
 * Long names: a project name, a voice profile and a workspace directory that
 * each run past their column truncate in place, with the whole value in the
 * tooltip.
 */
export const LongContent: Story = {
  args: {
    startHereDismissed: true,
    workspace: {
      location: "/fakehome/Library/Application Support/kapi/workspaces/acme-documentation-team",
      read_only: false,
      projects: [
        {
          ...SEVERAL_ROWS[0],
          name: "Acme Help Center and Developer Documentation Portal (2027 edition)",
          voice: "acme-developer-documentation-voice",
          context_files: {
            files: ["kapi/terms.yaml"],
            command: "kapi store import",
            message:
              "this project carries context files (kapi/terms.yaml) and its store holds no context yet. They are read by `kapi store import`, and until then nothing in them is in force",
          },
        },
        ...SEVERAL_ROWS.slice(1),
      ],
    },
  },
};

/**
 * A .kpz outside every project was replaced on disk while it held an edit:
 * its other version waits on the home to be rebased or discarded.
 */
export const WithDocumentConflict: Story = {
  args: {
    workspace: workspaceOf(ONE_PROJECT_ROWS),
    startHereDismissed: true,
    documentConflicts: [
      {
        ...documentConflict,
        doc: "/fakehome/handoff/loose.kpz!messages.json",
      },
    ],
  },
};

/**
 * Two worktrees of one repository are one project with two places to open it
 * from; a checkout that went away is shown as missing rather than dropped; and
 * a project only ever opened on another machine still opens, on its context.
 */
export const CheckoutsAndLosses: Story = {
  args: {
    startHereDismissed: true,
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

/** The workspace could not be opened at all. */
export const WorkspaceUnreadable: Story = {
  args: {
    startHereDismissed: true,
    workspace: null,
    workspaceError: new Error("workspace: /fakehome/Dropbox/kapi is inside a synchronized folder"),
  },
};
