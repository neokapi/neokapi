import type { Meta, StoryObj } from "@storybook/react-vite";
import { InviteManager } from "../../components/InviteManager";
import { withProviders } from "../decorators";
import { sampleWorkspace, viewerWorkspace } from "./fixtures";

const meta: Meta<typeof InviteManager> = {
  title: "Workspace/Access/InviteManager",
  component: InviteManager,
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
type Story = StoryObj<typeof InviteManager>;

/** Owner view with full access. */
export const OwnerView: Story = {
  args: { workspace: sampleWorkspace },
};

/** Viewer — component returns null since role is not owner/admin. */
export const ViewerHidden: Story = {
  args: { workspace: viewerWorkspace },
};
