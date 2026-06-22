import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { ToolboxPage } from "../components/ToolboxPage";

/**
 * The Toolbox hosts single Tools and saved Flows as two route-driven tabs.
 * These stories use placeholder panels in place of the real ToolRunnerPage /
 * FlowsPage so the tab chrome can be reviewed in isolation.
 */
const meta: Meta<typeof ToolboxPage> = {
  title: "Components/ToolboxPage",
  component: ToolboxPage,
};

export default meta;
type Story = StoryObj<typeof ToolboxPage>;

function Panel({ label }: { label: string }) {
  return (
    <div className="p-6">
      <h2 className="text-lg font-semibold">{label}</h2>
      <p className="mt-1 text-sm text-muted-foreground">Placeholder for the {label} surface.</p>
      <div className="mt-4 grid gap-3 sm:grid-cols-2">
        <div className="h-24 rounded-xl border border-dashed border-border" />
        <div className="h-24 rounded-xl border border-dashed border-border" />
      </div>
    </div>
  );
}

function Interactive({ initial }: { initial: "tools" | "flows" }) {
  const [tab, setTab] = useState<"tools" | "flows">(initial);
  return (
    <ToolboxPage
      tab={tab}
      onTabChange={(v) => setTab(v === "flows" ? "flows" : "tools")}
      tools={<Panel label="Tools" />}
      flows={<Panel label="Flows" />}
    />
  );
}

export const ToolsTab: Story = {
  render: () => <Interactive initial="tools" />,
};

export const FlowsTab: Story = {
  render: () => <Interactive initial="flows" />,
};
