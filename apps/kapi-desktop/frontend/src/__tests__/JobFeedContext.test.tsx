import { deferred, render, renderHook, screen, act, waitFor } from "./testUtils";
import { describe, it, expect, vi, beforeEach } from "vite-plus/test";
import { JobFeedProvider, useJobFeed } from "../context/JobFeedContext";
import { Events } from "@wailsio/runtime";
import { api } from "../hooks/useApi";

// The provider polls api.getRunState / api.getRunEvents as a safety net to
// settle jobs whose live "flow:event" stream went quiet. Mock both so tests
// can drive that reconciliation deterministically.
vi.mock("../hooks/useApi", () => ({
  api: {
    getRunState: vi.fn(),
    getRunEvents: vi.fn(),
  },
}));

// Helper to read context values.
function JobFeedReader({ onRead }: { onRead: (feed: ReturnType<typeof useJobFeed>) => void }) {
  const feed = useJobFeed();
  onRead(feed);
  return (
    <div>
      <span data-testid="job-count">{feed.jobs.length}</span>
      <span data-testid="has-active">{String(feed.hasActive)}</span>
      {feed.activeJob && <span data-testid="active-flow">{feed.activeJob.flowName}</span>}
      {feed.jobs.map((j) => (
        <span key={j.id} data-testid={`job-${j.id}`}>
          {j.status}
        </span>
      ))}
    </div>
  );
}

describe("JobFeedContext", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Default: backend still running, so the safety-net poll is a no-op and
    // jobs stay in whatever state the live path put them.
    vi.mocked(api.getRunState).mockResolvedValue("running");
    vi.mocked(api.getRunEvents).mockResolvedValue([]);
  });

  it("starts with empty state", () => {
    let feed: ReturnType<typeof useJobFeed> | undefined;
    render(
      <JobFeedProvider>
        <JobFeedReader
          onRead={(f) => {
            feed = f;
          }}
        />
      </JobFeedProvider>,
    );

    expect(feed?.jobs).toHaveLength(0);
    expect(feed?.hasActive).toBe(false);
    expect(feed?.activeJob).toBeNull();
  });

  it("provides clearJob and clearAll functions", () => {
    let feed: ReturnType<typeof useJobFeed> | undefined;
    render(
      <JobFeedProvider>
        <JobFeedReader
          onRead={(f) => {
            feed = f;
          }}
        />
      </JobFeedProvider>,
    );

    expect(typeof feed?.clearJob).toBe("function");
    expect(typeof feed?.clearAll).toBe("function");
  });

  it("shows no jobs initially", () => {
    render(
      <JobFeedProvider>
        <JobFeedReader onRead={() => {}} />
      </JobFeedProvider>,
    );

    expect(screen.getByTestId("job-count").textContent).toBe("0");
    expect(screen.getByTestId("has-active").textContent).toBe("false");
  });

  it("keeps the running project when another launch is requested", async () => {
    const { result } = renderHook(() => useJobFeed(), { wrapper: JobFeedProvider });
    await act(() => result.current.launchJob({ flowName: "check", tabID: "a" }, async () => {}));
    const other = vi.fn();
    await expect(
      result.current.launchJob({ flowName: "check", tabID: "b" }, other),
    ).rejects.toThrow();
    expect(other).not.toHaveBeenCalled();
    expect(result.current.activeJob).toMatchObject({
      tabID: "a",
      flowName: "check",
      stepSnapshots: [],
    });
  });

  it("keeps another project's terminal event out of the active run", async () => {
    const { result } = renderHook(() => useJobFeed(), { wrapper: JobFeedProvider });
    await act(() => result.current.launchJob({ tabID: "a", flowName: "check" }, async () => {}));
    const listener = vi.mocked(Events.On).mock.calls.find(([name]) => name === "flow:event")?.[1];
    expect(listener).toBeDefined();
    const send = (tab_id: string) =>
      act(() =>
        listener?.({ name: "flow:event", data: { tab_id, type: "complete", flow_id: "check" } }),
      );
    send("b");
    expect(result.current.activeJob?.tabID).toBe("a");
    send("a");
    expect(result.current.activeJob).toBeNull();
    expect(result.current.jobs[0].events).toHaveLength(1);
  });

  it("reports a rejected launch once and releases the runner", async () => {
    const { result } = renderHook(() => useJobFeed(), { wrapper: JobFeedProvider });
    const failed = async () => {
      throw new Error("no target languages configured");
    };
    await act(async () => {
      await expect(
        result.current.launchJob({ tabID: "a", flowName: "up" }, failed),
      ).rejects.toThrow();
    });
    expect(result.current.hasActive).toBe(false);
    expect(result.current.jobs[0]).toMatchObject({
      tabID: "a",
      status: "error",
      error: "no target languages configured",
    });
  });

  it("polls the accepted run and retains its terminal event when live delivery stops", async () => {
    const launch = deferred<void>();
    vi.mocked(api.getRunState).mockResolvedValue("complete");
    vi.mocked(api.getRunEvents).mockResolvedValue([
      { type: "complete", tab_id: "a", flow_id: "check", duration_ms: 4200 },
    ]);
    const { result } = renderHook(() => useJobFeed(), { wrapper: JobFeedProvider });
    // The provider asks once on load whether the backend is already running
    // something; the reconciliation poll must not start before the launch is
    // accepted.
    await waitFor(() => expect(api.getRunState).toHaveBeenCalledTimes(1));
    let pending: Promise<void>;
    act(() => {
      pending = result.current.launchJob({ tabID: "a", flowName: "check" }, () => launch.promise);
    });
    expect(api.getRunState).toHaveBeenCalledTimes(1);
    await act(async () => {
      launch.resolve();
      await pending;
    });
    await waitFor(() => expect(result.current.activeJob).toBeNull());
    expect(result.current.jobs[0]).toMatchObject({ status: "complete", durationMs: 4200 });
  });

  it("picks up a run the backend is executing when the page loads", async () => {
    vi.mocked(api.getRunState).mockResolvedValue("running");
    vi.mocked(api.getRunEvents).mockResolvedValue([
      { tab_id: "t1", type: "state", flow_id: "converge", message: "running" },
      { tab_id: "t1", type: "progress", flow_id: "converge", file_index: 1, file_count: 3 },
    ]);
    const { result } = renderHook(() => useJobFeed(), { wrapper: JobFeedProvider });
    await waitFor(() => expect(result.current.activeJob).not.toBeNull());
    expect(result.current.activeJob).toMatchObject({
      flowName: "converge",
      tabID: "t1",
      status: "running",
    });
  });

  it("starts no job when the backend runs nothing", async () => {
    vi.mocked(api.getRunState).mockResolvedValue("idle");
    vi.mocked(api.getRunEvents).mockResolvedValue([
      { tab_id: "t1", type: "complete", flow_id: "converge" },
    ]);
    const { result } = renderHook(() => useJobFeed(), { wrapper: JobFeedProvider });
    await act(async () => {});
    expect(result.current.jobs).toHaveLength(0);
  });
});
