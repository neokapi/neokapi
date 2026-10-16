import type { Meta, StoryObj } from "@storybook/react-vite";
import { WorkspaceRules } from "../components/WorkspaceRules";
import { WORKSPACE_RULES } from "./fixtures/standings";

const meta: Meta<typeof WorkspaceRules> = {
  title: "Components/WorkspaceRules",
  component: WorkspaceRules,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div className="mx-auto max-w-3xl p-6">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof WorkspaceRules>;

/** Two rules applied to every project: a word from one project's review, a voice rule from a merge. */
export const WithRules: Story = { args: { rules: WORKSPACE_RULES } };
export const WithRulesDark: Story = { args: WithRules.args, globals: { theme: "dark" } };

/** No rule holds in every project yet, so the section is not drawn: this story is blank. */
export const Empty: Story = { args: { rules: [] } };
