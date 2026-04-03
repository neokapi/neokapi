import type { Meta, StoryObj } from "@storybook/react-vite";
import { RoleTemplateManager } from "../../components/RoleTemplateManager";
import { withProviders } from "../decorators";
import { sampleWorkspace, viewerWorkspace } from "./fixtures";

const meta: Meta<typeof RoleTemplateManager> = {
  title: "Workspace/Access/RoleTemplateManager",
  component: RoleTemplateManager,
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
type Story = StoryObj<typeof RoleTemplateManager>;

/** Owner view with full access. */
export const OwnerView: Story = {
  args: { workspace: sampleWorkspace },
};

/** Viewer — component returns null since role is not owner/admin. */
export const ViewerHidden: Story = {
  args: { workspace: viewerWorkspace },
};
