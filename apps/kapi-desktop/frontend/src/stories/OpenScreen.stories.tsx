import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { OpenScreen } from "../components/OpenScreen";
import { ONE_PROJECT_ROWS, SEVERAL_ROWS, workspaceOf } from "./fixtures/standings";
import type { WorkspaceProject } from "../types/api";

const meta: Meta<typeof OpenScreen> = {
  title: "Components/OpenScreen",
  component: OpenScreen,
  tags: ["autodocs"],
  args: {
    onChooseFolder: fn(),
    onOpenPath: fn(),
    onTrySample: fn(),
    onOpenSettings: fn(),
  },
  parameters: {
    layout: "fullscreen",
  },
  decorators: [
    (Story) => (
      <div style={{ height: 720 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof OpenScreen>;

/** First launch: nothing has been opened on this machine. */
export const FirstLaunch: Story = {
  args: { workspace: workspaceOf([]) },
};

/** One folder opened before. */
export const OneRecentFolder: Story = {
  args: { workspace: workspaceOf(ONE_PROJECT_ROWS) },
};

/** Several folders, newest first. */
export const RecentFolders: Story = {
  args: { workspace: workspaceOf(SEVERAL_ROWS) },
};

const LONG_ROW: WorkspaceProject = {
  key: "prj_long",
  name: "customer-facing-documentation-and-release-notes-for-the-enterprise-edition",
  last_active: "2026-10-29T08:00:00Z",
  checkouts: [
    {
      path: "/fakehome/src/clients/northwind/customer-facing-documentation-and-release-notes-for-the-enterprise-edition",
      recipe:
        "/fakehome/src/clients/northwind/customer-facing-documentation-and-release-notes-for-the-enterprise-edition/kapi.yaml",
      missing: false,
    },
  ],
};

/** A folder whose name runs past the row. */
export const LongFolderName: Story = {
  args: { workspace: workspaceOf([LONG_ROW, ...ONE_PROJECT_ROWS]) },
};

/** The workspace is still being read. */
export const Loading: Story = {
  args: { workspace: null },
};
