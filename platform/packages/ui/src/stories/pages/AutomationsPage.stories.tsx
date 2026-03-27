import type { Meta, StoryObj } from "@storybook/react-vite";
import { AutomationsPage } from "../../components/AutomationsPage";
import { withProviders } from "../decorators";

const meta: Meta<typeof AutomationsPage> = {
  title: "Pages/Automation/AutomationsPage",
  component: AutomationsPage,
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
type Story = StoryObj<typeof AutomationsPage>;

export const Default: Story = {
  args: {
    workspaceSlug: "demo",
    projectId: "proj-demo-1",
  },
};
