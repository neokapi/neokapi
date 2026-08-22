import { describe, it, expect, vi } from "vite-plus/test";
import type { ReactNode } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider } from "../context/ApiContext";
import { WorkspaceProvider } from "../context/WorkspaceContext";
import { TaskBoard } from "../components/TaskBoard";
import { TaskIndicator } from "../components/ActivityTaskIndicators";
import { ConnectorsPanel } from "../components/ConnectorsPanel";
import { AutomationHistory } from "../components/AutomationHistory";
import { ExperimentsView } from "../context-hub/experiments/ExperimentsView";
import { sampleChangesets } from "../stories/voiceHubFixtures";
import type { ApiAdapter } from "../api/adapter";
import type {
  AutomationHistoryEntry,
  ConnectorSyncStatus,
  TaskInfo,
  Workspace,
} from "../types/api";

/**
 * The platform surfaces read their totals, filters and pages from the server
 * rather than deriving them from whatever the first request happened to
 * return. These tests pin that: a component handed one page must still report
 * the whole set, and a batch endpoint must be called once for the panel.
 */

const workspace: Workspace = {
  id: "ws-1",
  name: "Demo",
  slug: "demo",
  description: "",
  logo_url: "",
  type: "personal",
  role: "owner",
};

function mockAdapter(overrides: Partial<ApiAdapter> = {}): ApiAdapter {
  return overrides as unknown as ApiAdapter;
}

function renderWithProviders(ui: ReactNode, adapter: ApiAdapter) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <ApiProvider adapter={adapter}>
        <WorkspaceProvider initialWorkspace={workspace}>{ui}</WorkspaceProvider>
      </ApiProvider>
    </QueryClientProvider>,
  );
}

function makeTask(overrides: Partial<TaskInfo> = {}): TaskInfo {
  return {
    id: "task-1",
    workspace_id: "ws-1",
    project_id: "proj-1",
    stream: "",
    type: "translate",
    status: "open",
    priority: "normal",
    title: "Translate homepage",
    description: "",
    assignee_id: "user-2",
    created_by: "user-1",
    completed_by: "",
    data: {},
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// TaskBoard column headers
// ---------------------------------------------------------------------------

describe("TaskBoard column counts", () => {
  it("wears the server's per-status totals, not the loaded page's length", async () => {
    const user = userEvent.setup();
    render(
      <TaskBoard
        tasks={[makeTask({ id: "t1", status: "open" })]}
        statusCounts={{ open: 42, in_progress: 7, completed: 300, cancelled: 0 }}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Board" }));

    expect(screen.getByText("Open (42)")).toBeInTheDocument();
    expect(screen.getByText("In Progress (7)")).toBeInTheDocument();
    expect(screen.getByText("Completed (300)")).toBeInTheDocument();
  });

  it("falls back to the page when the counts have not arrived", async () => {
    const user = userEvent.setup();
    render(<TaskBoard tasks={[makeTask({ id: "t1", status: "open" })]} />);

    await user.click(screen.getByRole("button", { name: "Board" }));

    expect(screen.getByText("Open (1)")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Task badge
// ---------------------------------------------------------------------------

describe("TaskIndicator badge", () => {
  it("shows the server count rather than the size of the dropdown page", () => {
    render(<TaskIndicator tasks={[makeTask({ id: "t1" })]} count={23} />);
    expect(screen.getByText("23")).toBeInTheDocument();
  });

  it("caps the rendered badge at 99+", () => {
    render(<TaskIndicator tasks={[makeTask({ id: "t1" })]} count={512} />);
    expect(screen.getByText("99+")).toBeInTheDocument();
  });

  it("counts the actionable page when no server count is given", () => {
    render(
      <TaskIndicator
        tasks={[
          makeTask({ id: "t1", status: "open" }),
          makeTask({ id: "t2", status: "completed" }),
        ]}
      />,
    );
    expect(screen.getByText("1")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Connectors panel
// ---------------------------------------------------------------------------

function syncStatus(id: string, overrides: Partial<ConnectorSyncStatus> = {}): ConnectorSyncStatus {
  return {
    connectorId: id,
    lastSync: "2026-03-20T10:30:00Z",
    itemCount: 3,
    fileCount: 3,
    wordCount: 100,
    pendingPull: 0,
    pendingPush: 0,
    errors: [],
    ...overrides,
  };
}

describe("ConnectorsPanel status reads", () => {
  it("reads every connector's state in one batch call", async () => {
    const getConnectorStatuses = vi.fn().mockResolvedValue({
      statuses: {
        "conn-1": syncStatus("conn-1", { itemCount: 12 }),
        "conn-2": syncStatus("conn-2", { itemCount: 4, pendingPush: 2 }),
      },
      unknown: [],
    });
    const adapter = mockAdapter({
      listConnectors: vi.fn().mockResolvedValue([
        { id: "conn-1", name: "Site", category: "cms" },
        { id: "conn-2", name: "Design", category: "design" },
      ]),
      getConnectorStatuses,
    });

    renderWithProviders(<ConnectorsPanel workspaceSlug="demo" projectId="proj-1" />, adapter);

    await waitFor(() => expect(screen.getByText("12 items")).toBeInTheDocument());
    expect(screen.getByText("2 to push")).toBeInTheDocument();

    expect(getConnectorStatuses).toHaveBeenCalledTimes(1);
    expect(getConnectorStatuses).toHaveBeenCalledWith("demo", ["conn-1", "conn-2"], {
      probe: true,
    });
  });

  it("chunks past the endpoint's 100-id cap", async () => {
    const ids = Array.from({ length: 150 }, (_, i) => `conn-${i}`);
    const getConnectorStatuses = vi
      .fn()
      .mockImplementation(async (_ws: string, batch: string[]) => ({
        statuses: Object.fromEntries(batch.map((id) => [id, syncStatus(id)])),
        unknown: [],
      }));
    const adapter = mockAdapter({
      listConnectors: vi
        .fn()
        .mockResolvedValue(ids.map((id) => ({ id, name: id, category: "cms" }))),
      getConnectorStatuses,
    });

    renderWithProviders(<ConnectorsPanel workspaceSlug="demo" projectId="proj-1" />, adapter);

    await waitFor(() => expect(getConnectorStatuses).toHaveBeenCalledTimes(2));
    expect(getConnectorStatuses.mock.calls[0][1]).toHaveLength(100);
    expect(getConnectorStatuses.mock.calls[1][1]).toHaveLength(50);
  });

  it("re-reads the batch after a fetch, not just that connector's own entry", async () => {
    const user = userEvent.setup();
    const getConnectorStatuses = vi
      .fn()
      .mockResolvedValue({ statuses: { "conn-1": syncStatus("conn-1") }, unknown: [] });
    const adapter = mockAdapter({
      listConnectors: vi.fn().mockResolvedValue([{ id: "conn-1", name: "Site", category: "cms" }]),
      getConnectorStatuses,
      fetchConnector: vi.fn().mockResolvedValue({ items_fetched: 3 }),
    });

    renderWithProviders(<ConnectorsPanel workspaceSlug="demo" projectId="proj-1" />, adapter);

    await waitFor(() => expect(getConnectorStatuses).toHaveBeenCalledTimes(1));

    await user.click(screen.getByTestId("connector-fetch-conn-1"));

    // The panel's status lives under one batch key: an invalidation scoped to
    // the single connector id would never reach it.
    await waitFor(() => expect(getConnectorStatuses).toHaveBeenCalledTimes(2));
  });

  it("says so for a connector the batch could not answer for", async () => {
    const adapter = mockAdapter({
      listConnectors: vi.fn().mockResolvedValue([{ id: "conn-1", name: "Site", category: "cms" }]),
      getConnectorStatuses: vi.fn().mockResolvedValue({ statuses: {}, unknown: ["conn-1"] }),
    });

    renderWithProviders(<ConnectorsPanel workspaceSlug="demo" projectId="proj-1" />, adapter);

    await waitFor(() => expect(screen.getByText("Status unavailable")).toBeInTheDocument());
  });
});

// ---------------------------------------------------------------------------
// Automation history pagination
// ---------------------------------------------------------------------------

function historyEntry(id: string): AutomationHistoryEntry {
  return {
    id,
    rule_id: "rule-1",
    event_id: `evt-${id}`,
    status: "success",
    started_at: "2026-03-20T10:30:00Z",
    ended_at: "2026-03-20T10:30:01Z",
  };
}

describe("AutomationHistory pagination", () => {
  it("pages forward on the server's opaque cursor and back again", async () => {
    const user = userEvent.setup();
    const listAutomationHistory = vi
      .fn()
      .mockImplementation(async (_ws: string, _pid: string, opts?: { cursor?: string }) =>
        opts?.cursor === "cursor-2"
          ? { entries: [historyEntry("h2")] }
          : { entries: [historyEntry("h1")], next_cursor: "cursor-2" },
      );
    const adapter = mockAdapter({ listAutomationHistory });

    renderWithProviders(<AutomationHistory workspaceSlug="demo" projectId="proj-1" />, adapter);

    await waitFor(() => expect(screen.getByText("Event: evt-h1")).toBeInTheDocument());
    expect(screen.getByTestId("automation-history-newer")).toBeDisabled();

    await user.click(screen.getByTestId("automation-history-older"));
    await waitFor(() => expect(screen.getByText("Event: evt-h2")).toBeInTheDocument());
    expect(listAutomationHistory).toHaveBeenLastCalledWith("demo", "proj-1", {
      limit: 25,
      cursor: "cursor-2",
    });
    // The last page offers no way further down.
    expect(screen.getByTestId("automation-history-older")).toBeDisabled();

    await user.click(screen.getByTestId("automation-history-newer"));
    await waitFor(() => expect(screen.getByText("Event: evt-h1")).toBeInTheDocument());
  });

  it("offers no pager when the first page is the whole history", async () => {
    const adapter = mockAdapter({
      listAutomationHistory: vi.fn().mockResolvedValue({ entries: [historyEntry("h1")] }),
    });

    renderWithProviders(<AutomationHistory workspaceSlug="demo" projectId="proj-1" />, adapter);

    await waitFor(() => expect(screen.getByText("Event: evt-h1")).toBeInTheDocument());
    expect(screen.queryByTestId("automation-history-older")).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Experiments grouping
// ---------------------------------------------------------------------------

describe("ExperimentsView group counts", () => {
  it("reports each bucket's workspace total, not the filtered list's length", async () => {
    const adapter = mockAdapter({
      listConcepts: vi.fn().mockResolvedValue({ concepts: [], total_count: 0 }),
      listMarkets: vi.fn().mockResolvedValue([]),
      listChangesets: vi.fn().mockResolvedValue(sampleChangesets),
      getChangesetCounts: vi
        .fn()
        .mockResolvedValue({ total: 61, by_status: { draft: 9, in_review: 40, merged: 12 } }),
      listMembers: vi.fn().mockResolvedValue([]),
    });

    renderWithProviders(<ExperimentsView onOpenExperiment={vi.fn()} />, adapter);

    await waitFor(() => expect(screen.getByText("In review")).toBeInTheDocument());
    expect(screen.getByText("40")).toBeInTheDocument();
    expect(screen.getByText("12")).toBeInTheDocument();
  });
});
