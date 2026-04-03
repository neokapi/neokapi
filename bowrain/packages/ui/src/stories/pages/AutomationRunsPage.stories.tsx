import type { Meta, StoryObj } from "@storybook/react-vite";
import { AutomationRunsPage } from "../../components/AutomationRunsPage";
import { withProviders } from "../decorators";

const meta: Meta<typeof AutomationRunsPage> = {
  title: "Pages/Automation/AutomationRunsPage",
  component: AutomationRunsPage,
  tags: ["autodocs"],
  decorators: [
    withProviders,
    (Story) => (
      <div style={{ maxWidth: 960, height: 600, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof AutomationRunsPage>;

export const Empty: Story = {
  args: {
    projectId: "proj-demo-1",
  },
};
