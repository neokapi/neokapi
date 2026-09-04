import { describe, it, expect, vi, beforeEach, afterEach } from "vite-plus/test";
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { AutomationRunsPage } from "../components/AutomationRunsPage";
import { ApiProvider } from "../context/ApiContext";
import { WorkspaceProvider } from "../context/WorkspaceContext";
import type { ApiAdapter } from "../api/adapter";
import type { AutomationRun, AutomationStep, Workspace } from "../types/api";

type Listener = (e: MessageEvent) => void;

class FakeEventSource {
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSED = 2;
  static instances: FakeEventSource[] = [];
  url: string;
  readyState = FakeEventSource.CONNECTING;
  onopen: (() => void) | null = null;
  onerror: (() => void) | null = null;
  listeners = new Map<string, Set<Listener>>();
  closed = false;

  constructor(url: string) {
    this.url = url;
    FakeEventSource.instances.push(this);
  }
  addEventListener(type: string, listener: Listener) {
    let s = this.listeners.get(type);
    if (!s) {
      s = new Set();
      this.listeners.set(type, s);
    }
    s.add(listener);
  }
  close() {
    this.closed = true;
    this.readyState = FakeEventSource.CLOSED;
  }
  open() {
    this.readyState = FakeEventSource.OPEN;
    this.onopen?.();
  }
  emit(type: string, data: unknown) {
    for (const l of this.listeners.get(type) ?? []) {
      l({ data: JSON.stringify(data) } as MessageEvent);
    }
  }
}

const workspace: Workspace = {
  id: "ws-1",
  name: "Acme",
  slug: "acme",
  description: "",
  logo_url: "",
  type: "team",
};

const runningRun: AutomationRun = {
  id: "run-1",
  project_id: "p1",
  trigger_type: "connector.push.completed",
  trigger_id: "evt-1",
  trigger_data: { items: "en.json" },
  status: "running",
  step_count: 1,
  done_count: 0,
  started_at: new Date().toISOString(),
};

const runningStep: AutomationStep = {
  id: "step-1",
  run_id: "run-1",
  rule_name: "checks-on-push",
  action_type: "run_flow",
  status: "running",
  total_jobs: 0,
  done_jobs: 0,
  started_at: new Date().toISOString(),
};

function stubAdapter() {
  const listAutomationRuns = vi.fn(async () => [runningRun]);
  const getAutomationRun = vi.fn(async () => ({ run: runningRun, steps: [runningStep] }));
  const listStepLogs = vi.fn(async () => []);
  const adapter = {
    listAutomationRuns,
    getAutomationRun,
    listStepLogs,
    cancelAutomationRun: vi.fn(async () => {}),
  } as unknown as ApiAdapter;
  return { adapter, listAutomationRuns, getAutomationRun };
}

function renderPage(adapter: ApiAdapter, live: boolean) {
  const wrapper = ({ children }: { children: ReactNode }) => (
    <WorkspaceProvider initialWorkspace={workspace}>
      <ApiProvider adapter={adapter}>{children}</ApiProvider>
    </WorkspaceProvider>
  );
  return render(<AutomationRunsPage projectId="p1" live={live} />, { wrapper });
}

describe("AutomationRunsPage live stream", () => {
  beforeEach(() => {
    FakeEventSource.instances = [];
    vi.stubGlobal("EventSource", FakeEventSource as unknown as typeof EventSource);
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("renders the selected run from pushed frames and stops polling its steps", async () => {
    const user = userEvent.setup();
    const { adapter, getAutomationRun, listAutomationRuns } = stubAdapter();
    renderPage(adapter, true);

    await user.click(await screen.findByText("Content pushed"));

    // Selecting the run opens its stream; until it connects the steps come
    // from one poll.
    expect(FakeEventSource.instances).toHaveLength(1);
    const es = FakeEventSource.instances[0];
    expect(es.url).toBe("/api/v1/acme/p1/automations/runs/run-1/events");
    expect(await screen.findByText("checks-on-push")).toBeInTheDocument();
    expect(getAutomationRun).toHaveBeenCalledTimes(1);
    expect(screen.getByText("running")).toBeInTheDocument();
    expect(screen.getByText("●")).toBeInTheDocument();

    act(() => es.open());

    // The step finishing arrives as a push and is drawn at once.
    act(() =>
      es.emit("message", {
        type: "step.finished",
        run: { ...runningRun, done_count: 1 },
        steps: [{ ...runningStep, status: "completed", ended_at: new Date().toISOString() }],
      }),
    );
    expect(screen.getByText("✓")).toBeInTheDocument();
    expect(screen.getByText(/1\/1 steps/)).toBeInTheDocument();

    // The run settling updates its badge in the list from the same frame.
    act(() =>
      es.emit("message", {
        type: "run.finished",
        run: {
          ...runningRun,
          status: "completed",
          done_count: 1,
          ended_at: new Date().toISOString(),
        },
        steps: [{ ...runningStep, status: "completed", ended_at: new Date().toISOString() }],
      }),
    );
    expect(screen.getByText("completed")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Cancel" })).not.toBeInTheDocument();

    // The stream closing on the settled run refreshes the list once.
    const listCallsBeforeDone = listAutomationRuns.mock.calls.length;
    act(() => es.emit("done", {}));
    expect(listAutomationRuns.mock.calls.length).toBe(listCallsBeforeDone + 1);

    // No step poll ran while the stream was open.
    expect(getAutomationRun).toHaveBeenCalledTimes(1);
  });

  it("opens no stream when live is off", async () => {
    const user = userEvent.setup();
    const { adapter, getAutomationRun } = stubAdapter();
    renderPage(adapter, false);

    await user.click(await screen.findByText("Content pushed"));
    expect(await screen.findByText("checks-on-push")).toBeInTheDocument();
    expect(FakeEventSource.instances).toHaveLength(0);
    expect(getAutomationRun).toHaveBeenCalledTimes(1);
  });
});
