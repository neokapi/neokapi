import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { AutomationRuleEditor } from "../../components/AutomationRuleEditor";
import { withProviders } from "../decorators";
import { sampleAutomationRules } from "../fixtures";

const meta: Meta<typeof AutomationRuleEditor> = {
  title: "Pages/AutomationRuleEditor",
  component: AutomationRuleEditor,
  tags: ["autodocs"],
  decorators: [withProviders],
};

export default meta;
type Story = StoryObj<typeof AutomationRuleEditor>;

export const NewRule: Story = {
  args: {
    open: true,
    onOpenChange: fn(),
    workspaceSlug: "demo",
    projectId: "proj-demo-1",
    onSave: fn(),
  },
};

export const EditExistingRule: Story = {
  args: {
    open: true,
    onOpenChange: fn(),
    workspaceSlug: "demo",
    projectId: "proj-demo-1",
    rule: sampleAutomationRules[0],
    onSave: fn(),
  },
};

export const SavingState: Story = {
  args: {
    open: true,
    onOpenChange: fn(),
    workspaceSlug: "demo",
    projectId: "proj-demo-1",
    rule: sampleAutomationRules[0],
    onSave: fn(),
    saving: true,
  },
};

function InteractiveWrapper() {
  const [open, setOpen] = useState(true);
  return (
    <div>
      <button onClick={() => setOpen(true)} className="px-3 py-1.5 text-sm border rounded">
        Open Editor
      </button>
      <AutomationRuleEditor
        open={open}
        onOpenChange={setOpen}
        workspaceSlug="demo"
        projectId="proj-demo-1"
        onSave={(data) => {
          console.log("Saved:", data);
          setOpen(false);
        }}
      />
    </div>
  );
}

export const Interactive: Story = {
  render: () => <InteractiveWrapper />,
};
