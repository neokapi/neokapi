import type { Meta, StoryObj } from "@storybook/react-vite";
import { ProjectMemberManager } from "../../components/ProjectMemberManager";
import { withProviders } from "../decorators";
import { sampleWorkspace, viewerWorkspace } from "./fixtures";

const meta: Meta<typeof ProjectMemberManager> = {
  title: "Components/ProjectMemberManager",
  component: ProjectMemberManager,
  tags: ["autodocs"],
  decorators: [
    withProviders,
    (Story) => (
      <div style={{ maxWidth: 800, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ProjectMemberManager>;

/** Owner view with full access to manage project members. */
export const Default: Story = {
  args: {
    workspace: sampleWorkspace,
    projectId: "proj-1",
    projectLanguages: ["fr", "de", "ja", "es"],
  },
};

/** Viewer — component returns null since role is not owner/admin. */
export const ViewerHidden: Story = {
  args: {
    workspace: viewerWorkspace,
    projectId: "proj-1",
    projectLanguages: ["fr", "de", "ja", "es"],
  },
};
