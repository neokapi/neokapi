import type { Meta, StoryObj } from "@storybook/react-vite";
import { AutomationHistory } from "../../components/AutomationHistory";
import { withProviders } from "../decorators";

const meta: Meta<typeof AutomationHistory> = {
  title: "Pages/Automation/AutomationHistory",
  component: AutomationHistory,
  tags: ["autodocs"],
  decorators: [
    withProviders,
    (Story) => (
      <div style={{ maxWidth: 720, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof AutomationHistory>;

export const Default: Story = {
  args: {
    workspaceSlug: "demo",
    projectId: "proj-demo-1",
    ruleNames: {
      "rule-1": "Auto-translate on upload",
      "rule-2": "QA check after translation",
      "rule-3": "Notify on sync",
    },
  },
};

export const WithoutRuleNames: Story = {
  args: {
    workspaceSlug: "demo",
    projectId: "proj-demo-1",
  },
};
