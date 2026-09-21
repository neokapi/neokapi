import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { WorkspaceProjectRow } from "../components/WorkspaceProjectRow";

const meta: Meta<typeof WorkspaceProjectRow> = {
  title: "Components/WorkspaceProjectRow",
  component: WorkspaceProjectRow,
  tags: ["autodocs"],
  args: {
    onOpenCheckout: fn(),
    onOpenContext: fn(),
    onRemove: fn(),
  },
};

export default meta;
type Story = StoryObj<typeof WorkspaceProjectRow>;

/** One checkout: the row opens it. */
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

/** Several worktrees of one repository: the reader picks which to open. */
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

/** The folder went away. The project and its context stay. */
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
