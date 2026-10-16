import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { WorkspaceProjectRow } from "../components/WorkspaceProjectRow";
import { SEVERAL_ROWS } from "./fixtures/standings";

const meta: Meta<typeof WorkspaceProjectRow> = {
  title: "Components/WorkspaceProjectRow",
  component: WorkspaceProjectRow,
  tags: ["autodocs"],
  args: {
    onOpenCheckout: fn(),
    onOpenContext: fn(),
    onRemove: fn(),
  },
  decorators: [
    (Story) => (
      <div className="mx-auto max-w-4xl p-6">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof WorkspaceProjectRow>;

const dark = { theme: "dark" };

/** One checkout: Open opens it, and its path is the tooltip on the name. */
export const OneCheckout: Story = {
  args: {
    project: {
      key: "prj_kapimart",
      name: "KapiMart",
      last_active: "2026-09-21T08:05:00Z",
      checkouts: [
        {
          path: "/fakehome/src/kapimart",
          recipe: "/fakehome/src/kapimart/kapi.yaml",
          missing: false,
        },
      ],
    },
  },
};

/** Several worktrees of one repository: Open opens the first, the menu holds the other. */
export const SeveralCheckouts: Story = {
  args: {
    project: {
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
  },
};

/** The folder went away. The project and its context stay, and it opens on its context. */
export const MissingCheckout: Story = {
  args: {
    project: {
      key: "prj_handbook",
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
  },
};

/** Registered from another machine: the context opens without the files. */
export const NoCheckoutHere: Story = {
  args: {
    project: {
      key: "prj_fieldguide",
      name: "Field Guide",
      last_active: "2026-09-10T16:45:00Z",
      checkouts: [],
    },
  },
};

/** The columns: the voice, the rules in force, how far the context reaches, and when. */
export const WithContext: Story = {
  args: { project: SEVERAL_ROWS[0] },
};
export const WithContextDark: Story = { args: WithContext.args, globals: dark };

/** A project shared through git, with two worktrees here. */
export const SharedThroughGit: Story = {
  args: { project: SEVERAL_ROWS[1] },
};

/** A project connected to a Bowrain server, with no checkout here and no voice bound. */
export const OnBowrain: Story = {
  args: { project: SEVERAL_ROWS[2] },
};

/** A checkout holding context files its store has never read. */
export const UnreadContextFiles: Story = {
  args: {
    project: {
      ...SEVERAL_ROWS[3],
      context_files: {
        files: ["kapi/terms.yaml"],
        command: "kapi store import",
        message:
          "this project carries context files (kapi/terms.yaml) and its store holds no context yet. They are read by `kapi store import`, and until then nothing in them is in force",
      },
    },
  },
};
export const UnreadContextFilesDark: Story = { args: UnreadContextFiles.args, globals: dark };
