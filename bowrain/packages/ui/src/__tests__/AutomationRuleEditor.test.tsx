import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import {
  ACTION_TYPES,
  AutomationRuleEditor,
  actionIsComplete,
} from "../components/AutomationRuleEditor";
import { ApiProvider } from "../context/ApiContext";
import type { ApiAdapter } from "../api/adapter";
import type { AutomationRule, FlowDefinitionInfo } from "../types/api";

const flows: FlowDefinitionInfo[] = [
  { id: "qa", name: "Quality Check", source: "built-in", nodes: [], edges: [] },
  { id: "mine", name: "Mine", source: "project", nodes: [], edges: [] },
];

function stubAdapter(available: FlowDefinitionInfo[]): ApiAdapter {
  return {
    listAutomationEvents: async () => [
      { type: "connector.pull.completed", description: "When content is pulled" },
    ],
    listFlowDefinitions: async () => available,
  } as unknown as ApiAdapter;
}

function rule(overrides: Partial<AutomationRule>): AutomationRule {
  return {
    id: "r1",
    project_id: "p1",
    name: "Checks on pull",
    trigger: "connector.pull.completed",
    conditions: [],
    actions: [{ Type: "run_flow", Config: { flow: "qa" } }],
    enabled: true,
    builtin: false,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

function renderEditor(existing: AutomationRule, available = flows) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>
      <ApiProvider adapter={stubAdapter(available)}>{children}</ApiProvider>
    </QueryClientProvider>
  );
  const onSave = vi.fn();
  render(
    <AutomationRuleEditor
      open
      onOpenChange={() => {}}
      workspaceSlug="ws"
      projectId="p1"
      rule={existing}
      onSave={onSave}
    />,
    { wrapper },
  );
  return onSave;
}

describe("AutomationRuleEditor run_flow", () => {
  it("offers only the action types the server runs", () => {
    expect(ACTION_TYPES).toContain("run_flow");
    expect(ACTION_TYPES).not.toContain("webhook");
  });

  it("shows the flow picker with the rule's flow selected", async () => {
    renderEditor(rule({}));
    expect(await screen.findByText("Quality Check (built-in)")).toBeDefined();
    expect(screen.getByRole("button", { name: "Update" })).toHaveProperty("disabled", false);
  });

  it("keeps the rule unsaveable until a run_flow action names a flow", async () => {
    renderEditor(rule({ actions: [{ Type: "run_flow", Config: { flow: "" } }] }));
    expect(await screen.findByText("Select a flow")).toBeDefined();
    expect(screen.getByRole("button", { name: "Update" })).toHaveProperty("disabled", true);
  });

  it("says so when the project has no flows to pick", async () => {
    renderEditor(rule({ actions: [{ Type: "run_flow", Config: { flow: "" } }] }), []);
    expect(await screen.findByText("Select a flow")).toBeDefined();
    expect(screen.getByRole("button", { name: "Update" })).toHaveProperty("disabled", true);
  });

  it("treats every other action as complete without a flow", () => {
    expect(actionIsComplete({ Type: "notify", Config: {} })).toBe(true);
    expect(actionIsComplete({ Type: "run_flow", Config: {} })).toBe(false);
    expect(actionIsComplete({ Type: "run_flow", Config: { flow: "  " } })).toBe(false);
    expect(actionIsComplete({ Type: "run_flow", Config: { flow: "qa" } })).toBe(true);
  });
});
