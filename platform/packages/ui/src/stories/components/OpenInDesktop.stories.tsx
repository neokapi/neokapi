import type { Meta, StoryObj } from "@storybook/react-vite";
import { OpenInDesktop } from "../../components/OpenInDesktop";

const meta: Meta<typeof OpenInDesktop> = {
  title: "Misc/OpenInDesktop",
  component: OpenInDesktop,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 800, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof OpenInDesktop>;

export const Default: Story = {
  args: {
    projectId: "proj-1",
    serverURL: "http://localhost:8080",
    workspaceSlug: "acme",
  },
};
