import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { FlowsEmptyState } from "../../components/flows";

const meta: Meta<typeof FlowsEmptyState> = {
  title: "Flows/FlowsEmptyState",
  component: FlowsEmptyState,
  tags: ["autodocs"],
  render: (args) => (
    <div className="max-w-xl">
      <FlowsEmptyState {...args} />
    </div>
  ),
};

export default meta;
type Story = StoryObj<typeof FlowsEmptyState>;

export const ProjectWithoutFlows: Story = {
  name: "Project (runs the default flow)",
  args: { projectMode: true, onCreate: fn() },
};

export const AdHocEmpty: Story = {
  name: "Ad-hoc (no flows yet)",
  args: { projectMode: false, onCreate: fn() },
};
