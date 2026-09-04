import { describe, it, expect, vi, beforeEach, afterEach } from "vite-plus/test";
import { act, renderHook } from "@testing-library/react";
import { useAutomationRunEvents } from "../hooks/useAutomationRunEvents";
import type { AutomationRun, AutomationStep } from "../types/api";

type Listener = (e: MessageEvent) => void;

/** A stand-in for the browser's EventSource that the test drives by hand. */
class FakeEventSource {
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSED = 2;
  static instances: FakeEventSource[] = [];
  url: string;
  withCredentials: boolean;
  readyState = FakeEventSource.CONNECTING;
  onopen: (() => void) | null = null;
  onerror: (() => void) | null = null;
  listeners = new Map<string, Set<Listener>>();
  closed = false;

  constructor(url: string, init?: { withCredentials?: boolean }) {
    this.url = url;
    this.withCredentials = init?.withCredentials ?? false;
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
    const raw = typeof data === "string" ? data : JSON.stringify(data);
    for (const l of this.listeners.get(type) ?? []) {
      l({ data: raw } as MessageEvent);
    }
  }
  /** The browser hard-closing the stream (server restart, session expiry). */
  hardClose() {
    this.readyState = FakeEventSource.CLOSED;
    this.onerror?.();
  }
}

function run(overrides: Partial<AutomationRun> = {}): AutomationRun {
  return {
    id: "run-1",
    project_id: "p1",
    trigger_type: "connector.push.completed",
    trigger_id: "evt-1",
    trigger_data: {},
    status: "running",
    step_count: 1,
    done_count: 0,
    started_at: "2026-09-05T10:00:00Z",
    ...overrides,
  };
}

function step(overrides: Partial<AutomationStep> = {}): AutomationStep {
  return {
    id: "step-1",
    run_id: "run-1",
    rule_name: "draft",
    action_type: "run_flow",
    status: "running",
    total_jobs: 0,
    done_jobs: 0,
    started_at: "2026-09-05T10:00:00Z",
    ...overrides,
  };
}

describe("useAutomationRunEvents", () => {
  beforeEach(() => {
    FakeEventSource.instances = [];
    vi.stubGlobal("EventSource", FakeEventSource as unknown as typeof EventSource);
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it("opens the run's stream with the session cookie and folds each frame", () => {
    const { result } = renderHook(() => useAutomationRunEvents("acme", "p1", "run-1"));

    expect(FakeEventSource.instances).toHaveLength(1);
    const es = FakeEventSource.instances[0];
    expect(es.url).toBe("/api/v1/acme/p1/automations/runs/run-1/events");
    expect(es.withCredentials).toBe(true);
    expect(result.current).toEqual({ run: null, steps: null, done: false, connected: false });

    act(() => es.open());
    expect(result.current.connected).toBe(true);

    act(() => es.emit("message", { type: "snapshot", run: run(), steps: [step()] }));
    expect(result.current.run?.status).toBe("running");
    expect(result.current.steps).toHaveLength(1);

    act(() =>
      es.emit("message", {
        type: "step.finished",
        run: run({ done_count: 1 }),
        steps: [step({ status: "completed" })],
      }),
    );
    expect(result.current.run?.done_count).toBe(1);
    expect(result.current.steps?.[0].status).toBe("completed");

    act(() =>
      es.emit("message", {
        type: "run.finished",
        run: run({ status: "completed", done_count: 1 }),
        steps: [step({ status: "completed" })],
      }),
    );
    expect(result.current.run?.status).toBe("completed");
    expect(result.current.done).toBe(false);

    act(() => es.emit("done", "{}"));
    expect(result.current.done).toBe(true);
    expect(result.current.connected).toBe(false);
    expect(es.closed).toBe(true);

    // The server closing the socket after done surfaces as an error; no reconnect.
    act(() => es.hardClose());
    expect(FakeEventSource.instances).toHaveLength(1);
  });

  it("drops a malformed frame and a frame without a run", () => {
    const { result } = renderHook(() => useAutomationRunEvents("acme", "p1", "run-1"));
    const es = FakeEventSource.instances[0];
    act(() => es.open());
    act(() => es.emit("message", "not json"));
    act(() => es.emit("message", { type: "snapshot" }));
    expect(result.current.run).toBeNull();
    expect(result.current.steps).toBeNull();
  });

  it("opens nothing when disabled or without a run, and closes when the run changes", () => {
    const { result, rerender } = renderHook(
      ({ runId, enabled }: { runId: string | null; enabled: boolean }) =>
        useAutomationRunEvents("acme", "p1", runId, enabled),
      { initialProps: { runId: "run-1", enabled: false } },
    );
    expect(FakeEventSource.instances).toHaveLength(0);
    expect(result.current.connected).toBe(false);

    rerender({ runId: null, enabled: true });
    expect(FakeEventSource.instances).toHaveLength(0);

    rerender({ runId: "run-1", enabled: true });
    expect(FakeEventSource.instances).toHaveLength(1);
    const first = FakeEventSource.instances[0];
    act(() => first.open());
    act(() => first.emit("message", { type: "snapshot", run: run(), steps: [step()] }));
    expect(result.current.run?.id).toBe("run-1");

    rerender({ runId: "run-2", enabled: true });
    expect(first.closed).toBe(true);
    expect(FakeEventSource.instances).toHaveLength(2);
    expect(FakeEventSource.instances[1].url).toBe("/api/v1/acme/p1/automations/runs/run-2/events");
    // The previous run's state does not bleed into the next run's stream.
    expect(result.current).toEqual({ run: null, steps: null, done: false, connected: false });
  });

  it("reconnects with backoff after a hard close, until done", () => {
    vi.useFakeTimers();
    const { result } = renderHook(() => useAutomationRunEvents("acme", "p1", "run-1"));
    const first = FakeEventSource.instances[0];
    act(() => first.open());
    expect(result.current.connected).toBe(true);

    act(() => first.hardClose());
    expect(result.current.connected).toBe(false);
    expect(FakeEventSource.instances).toHaveLength(1);

    act(() => vi.advanceTimersByTime(1000));
    expect(FakeEventSource.instances).toHaveLength(2);
    const second = FakeEventSource.instances[1];

    // A reconnect that fails before opening waits twice as long.
    act(() => second.hardClose());
    act(() => vi.advanceTimersByTime(1000));
    expect(FakeEventSource.instances).toHaveLength(2);
    act(() => vi.advanceTimersByTime(1000));
    expect(FakeEventSource.instances).toHaveLength(3);

    // A reconnect that opens resets the backoff.
    const third = FakeEventSource.instances[2];
    act(() => third.open());
    expect(result.current.connected).toBe(true);
    act(() => third.hardClose());
    act(() => vi.advanceTimersByTime(1000));
    expect(FakeEventSource.instances).toHaveLength(4);
  });

  it("closes the stream on unmount", () => {
    const { unmount } = renderHook(() => useAutomationRunEvents("acme", "p1", "run-1"));
    const es = FakeEventSource.instances[0];
    unmount();
    expect(es.closed).toBe(true);
  });
});
