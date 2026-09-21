import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { RemoveProjectDialog } from "../components/RemoveProjectDialog";

const meta: Meta<typeof RemoveProjectDialog> = {
  title: "Components/RemoveProjectDialog",
  component: RemoveProjectDialog,
  tags: ["autodocs"],
  args: {
    onClose: fn(),
    onConfirm: fn(),
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

export default meta;
type Story = StoryObj<typeof RemoveProjectDialog>;

/** The confirmation names the file that goes and the folders that stay. */
export const WithCheckouts: Story = {
  args: {
    removal: {
      key: "prj_kapimart",
      name: "KapiMart",
      store: "/fakehome/.local/share/kapi/workspaces/default/projects/prj_kapimart.db",
      checkouts: ["/fakehome/src/kapimart", "/fakehome/src/kapimart-release"],
    },
  },
};

/** A project with no copy on this machine: nothing on disk changes. */
export const NoCheckoutHere: Story = {
  args: {
    project: {
      key: "prj_fieldguide",
      name: "Field Guide",
      last_active: "2026-09-10T16:45:00Z",
      checkouts: [],
    },
    removal: {
      key: "prj_fieldguide",
      name: "Field Guide",
      store: "/fakehome/.local/share/kapi/workspaces/default/projects/prj_fieldguide.db",
      checkouts: [],
    },
  },
};
