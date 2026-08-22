import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { createElement } from "react";
import { ApiProvider, type ApiAdapter } from "@neokapi/ui";
import {
  planInvalidations,
  createEventInvalidator,
  useWorkspaceEvents,
  LIST_INVALIDATE_DEBOUNCE_MS,
  SSE_INITIAL_BACKOFF_MS,
  SSE_MAX_BACKOFF_MS,
  SSE_MAX_REFRESH_FAILURES,
  initialSseReconnectState,
  sseClosed,
  sseRefreshResult,
  sseShouldStop,
  sseOpened,
} from "./useWorkspaceEvents";

describe("planInvalidations → query-key mapping", () => {
  const cases: Array<{ type: string; expectIncludes: string[] }> = [
    { type: "block.updated", expectIncludes: ["project", "translationDashboard"] },
    { type: "editor.block.updated", expectIncludes: ["project", "translationDashboard"] },
    { type: "project.updated", expectIncludes: ["projects", "project"] },
    { type: "collection.created", expectIncludes: ["projects", "project"] },
    { type: "extraction.completed", expectIncludes: ["projects", "project"] },
    { type: "item.created", expectIncludes: ["project", "projects"] },
    { type: "stream.merged", expectIncludes: ["project"] },
    { type: "task.assigned", expectIncludes: ["tasks", "myTasks"] },
    { type: "member.added", expectIncludes: ["members", "project"] },
    { type: "voice.profile.updated", expectIncludes: ["voice-candidates", "voice-profiles"] },
    { type: "voice.drift", expectIncludes: ["voice-drift"] },
    { type: "connector.sync.completed", expectIncludes: ["connectors", "project"] },
    { type: "flow.completed", expectIncludes: ["flows", "automation-runs"] },
    {
      type: "convergence.run.completed",
      expectIncludes: ["activities", "convergenceRuns", "loopRollup", "translationDashboard"],
    },
    { type: "push.automations.completed", expectIncludes: ["flows", "automation-runs"] },
    { type: "version.created", expectIncludes: ["projects", "project"] },
    { type: "something.unknown", expectIncludes: ["activities", "auditlog"] },
  ];

  for (const tc of cases) {
    it(`plans ${tc.expectIncludes.join("+")} for "${tc.type}"`, () => {
      const plan = planInvalidations("acme", { type: tc.type });
      const keys = [...plan.immediate, ...plan.debounced];
      const roots = keys.map((k) => k[0]);
      for (const expected of tc.expectIncludes) {
        expect(roots).toContain(expected);
      }
      // Every key is workspace-scoped.
      for (const k of keys) {
        expect(k[1]).toBe("acme");
      }
    });
  }

  it("ignores presence frames entirely", () => {
    const plan = planInvalidations("acme", { type: "editor.presence.focus" });
    expect(plan.immediate).toHaveLength(0);
    expect(plan.debounced).toHaveLength(0);
  });

  it("narrows project-scoped keys to the event's project id", () => {
    const plan = planInvalidations("acme", { type: "block.updated", projectId: "p1" });
    expect(plan.immediate).toContainEqual(["project", "acme", "p1"]);
    expect(plan.debounced).toContainEqual(["translationDashboard", "acme", "p1"]);
    // No workspace-wide project prefix — other projects' views stay untouched.
    expect(plan.immediate).not.toContainEqual(["project", "acme"]);
  });

  it("falls back to workspace-wide keys without a project id", () => {
    const plan = planInvalidations("acme", { type: "block.updated" });
    expect(plan.immediate).toContainEqual(["project", "acme"]);
    expect(plan.debounced).toContainEqual(["translationDashboard", "acme"]);
  });

  it("routes convergence run events to the run + rollup keys, debounced", () => {
    const plan = planInvalidations("acme", {
      type: "convergence.run.completed",
      projectId: "p1",
    });
    expect(plan.immediate).toContainEqual(["activities", "acme"]);
    expect(plan.debounced).toContainEqual(["convergenceRuns", "acme", "p1"]);
    expect(plan.debounced).toContainEqual(["loopRollup", "acme"]);
    expect(plan.debounced).toContainEqual(["translationDashboard", "acme", "p1"]);
  });

  it("routes the projects list through the debounced bucket", () => {
    for (const type of ["project.updated", "item.created", "connector.sync.completed"]) {
      const plan = planInvalidations("acme", { type, projectId: "p1" });
      expect(plan.debounced).toContainEqual(["projects", "acme"]);
      expect(plan.immediate).not.toContainEqual(["projects", "acme"]);
    }
  });
});

describe("createEventInvalidator (debounce)", () => {
  function capture() {
    const keys: unknown[][] = [];
    const qc: Pick<QueryClient, "invalidateQueries"> = {
      invalidateQueries: (filters) => {
        keys.push((filters?.queryKey ?? []) as unknown[]);
        return Promise.resolve();
      },
    };
    return { qc, keys };
  }

  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("invalidates targeted keys immediately and lists only after the debounce", () => {
    const { qc, keys } = capture();
    const inv = createEventInvalidator(qc, "acme");

    inv.handle({ type: "item.created", projectId: "p1" });
    expect(keys).toContainEqual(["project", "acme", "p1"]);
    expect(keys).not.toContainEqual(["projects", "acme"]);

    vi.advanceTimersByTime(LIST_INVALIDATE_DEBOUNCE_MS);
    expect(keys).toContainEqual(["projects", "acme"]);
    expect(keys).toContainEqual(["translationDashboard", "acme", "p1"]);
    inv.dispose();
  });

  it("coalesces a burst of events into one trailing list invalidation", () => {
    const { qc, keys } = capture();
    const inv = createEventInvalidator(qc, "acme");

    // 50 per-block frames during a sync, 100ms apart.
    for (let i = 0; i < 50; i++) {
      inv.handle({ type: "block.updated", projectId: "p1" });
      vi.advanceTimersByTime(100);
    }
    const dashCountDuringBurst = keys.filter((k) => k[0] === "translationDashboard").length;
    expect(dashCountDuringBurst).toBe(0);

    // Trailing edge: exactly one dashboard invalidation after the burst quiets.
    vi.advanceTimersByTime(LIST_INVALIDATE_DEBOUNCE_MS);
    expect(keys.filter((k) => k[0] === "translationDashboard")).toHaveLength(1);
    // The targeted project detail fired for every frame (immediate bucket).
    expect(keys.filter((k) => k[0] === "project")).toHaveLength(50);
    inv.dispose();
  });

  it("debounces distinct keys independently", () => {
    const { qc, keys } = capture();
    const inv = createEventInvalidator(qc, "acme");

    inv.handle({ type: "block.updated", projectId: "p1" });
    inv.handle({ type: "block.updated", projectId: "p2" });
    vi.advanceTimersByTime(LIST_INVALIDATE_DEBOUNCE_MS);

    expect(keys).toContainEqual(["translationDashboard", "acme", "p1"]);
    expect(keys).toContainEqual(["translationDashboard", "acme", "p2"]);
    inv.dispose();
  });

  it("dispose cancels pending debounced invalidations", () => {
    const { qc, keys } = capture();
    const inv = createEventInvalidator(qc, "acme");

    inv.handle({ type: "item.created", projectId: "p1" });
    inv.dispose();
    vi.advanceTimersByTime(LIST_INVALIDATE_DEBOUNCE_MS * 2);
    expect(keys).not.toContainEqual(["projects", "acme"]);
  });
});

describe("useWorkspaceEvents (fake EventSource)", () => {
  type Listener = (e: MessageEvent) => void;
  let instances: FakeEventSource[] = [];

  class FakeEventSource {
    static CONNECTING = 0;
    static OPEN = 1;
    static CLOSED = 2;
    url: string;
    withCredentials: boolean;
    readyState = FakeEventSource.OPEN;
    onopen: (() => void) | null = null;
    onerror: (() => void) | null = null;
    listeners = new Map<string, Set<Listener>>();
    closed = false;

    constructor(url: string, init?: { withCredentials?: boolean }) {
      this.url = url;
      this.withCredentials = init?.withCredentials ?? false;
      instances.push(this);
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
    emit(type: string, data: unknown) {
      for (const l of this.listeners.get(type) ?? []) {
        l({ data: JSON.stringify(data) } as MessageEvent);
      }
    }
  }

  beforeEach(() => {
    instances = [];
    vi.stubGlobal("EventSource", FakeEventSource as unknown as typeof EventSource);
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  // The hook reads the api adapter (for the pre-reconnect session refresh)
  // from ApiContext; tests supply only the members they exercise.
  function wrapper(qc: QueryClient, api: Partial<ApiAdapter> = {}) {
    return ({ children }: { children: ReactNode }) =>
      createElement(
        QueryClientProvider,
        { client: qc },
        createElement(ApiProvider, { adapter: api as ApiAdapter }, children),
      );
  }

  /** Simulate the browser hard-closing the stream (server 401/restart). */
  function hardClose(inst: FakeEventSource) {
    inst.readyState = FakeEventSource.CLOSED;
    inst.onerror?.();
  }

  it("opens a workspace-scoped stream and invalidates on a change frame", () => {
    const qc = new QueryClient();
    const spy = vi.spyOn(qc, "invalidateQueries");

    renderHook(() => useWorkspaceEvents("acme", "proj-1"), { wrapper: wrapper(qc) });

    expect(instances).toHaveLength(1);
    expect(instances[0].url).toBe("/api/v1/acme/events?project=proj-1");
    expect(instances[0].withCredentials).toBe(true);

    instances[0].emit("change", { type: "block.updated", projectId: "proj-1" });

    // The targeted project-detail invalidation fires immediately.
    const invalidatedKeys = spy.mock.calls.map((c) => (c[0] as { queryKey: unknown[] }).queryKey);
    expect(invalidatedKeys).toContainEqual(["project", "acme", "proj-1"]);
  });

  it("uses the workspace-wide URL when no project is given", () => {
    const qc = new QueryClient();
    renderHook(() => useWorkspaceEvents("acme"), { wrapper: wrapper(qc) });
    expect(instances[0].url).toBe("/api/v1/acme/events");
  });

  it("closes the stream on unmount", () => {
    const qc = new QueryClient();
    const { unmount } = renderHook(() => useWorkspaceEvents("acme"), { wrapper: wrapper(qc) });
    expect(instances[0].closed).toBe(false);
    unmount();
    expect(instances[0].closed).toBe(true);
  });

  it("does nothing without a workspace slug", () => {
    const qc = new QueryClient();
    renderHook(() => useWorkspaceEvents(undefined), { wrapper: wrapper(qc) });
    expect(instances).toHaveLength(0);
  });

  describe("reconnect + session refresh", () => {
    beforeEach(() => {
      vi.useFakeTimers();
    });
    afterEach(() => {
      vi.useRealTimers();
    });

    it("refreshes the session before a manual reconnect", async () => {
      const refreshSession = vi.fn().mockResolvedValue(true);
      const qc = new QueryClient();
      renderHook(() => useWorkspaceEvents("acme"), {
        wrapper: wrapper(qc, { refreshSession }),
      });

      hardClose(instances[0]);
      // The reconnect waits out the backoff; nothing fires early.
      expect(instances).toHaveLength(1);
      expect(refreshSession).not.toHaveBeenCalled();

      await vi.advanceTimersByTimeAsync(SSE_INITIAL_BACKOFF_MS);
      expect(refreshSession).toHaveBeenCalledTimes(1);
      expect(instances).toHaveLength(2);
    });

    it("stops reconnecting after bounded consecutive refresh failures", async () => {
      const refreshSession = vi.fn().mockResolvedValue(false);
      const qc = new QueryClient();
      renderHook(() => useWorkspaceEvents("acme"), {
        wrapper: wrapper(qc, { refreshSession }),
      });

      // Drive close→backoff→refresh cycles; every refresh fails (dead session).
      for (let i = 0; i < SSE_MAX_REFRESH_FAILURES + 2; i++) {
        const current = instances[instances.length - 1];
        if (current.readyState !== FakeEventSource.CLOSED) hardClose(current);
        await vi.advanceTimersByTimeAsync(SSE_MAX_BACKOFF_MS);
      }

      // Exactly the bounded number of refresh attempts, then silence: the
      // final failed refresh does not open another doomed connection.
      expect(refreshSession).toHaveBeenCalledTimes(SSE_MAX_REFRESH_FAILURES);
      expect(instances).toHaveLength(SSE_MAX_REFRESH_FAILURES);
    });

    it("keeps reconnecting when the adapter has no refresh capability", async () => {
      const qc = new QueryClient();
      renderHook(() => useWorkspaceEvents("acme"), { wrapper: wrapper(qc) });

      for (let i = 0; i < SSE_MAX_REFRESH_FAILURES + 2; i++) {
        hardClose(instances[instances.length - 1]);
        await vi.advanceTimersByTimeAsync(SSE_MAX_BACKOFF_MS);
      }
      // No refresh seam → nothing to give up on: the pre-existing endless
      // backoff loop (desktop Bearer transport, older adapters) is preserved.
      expect(instances.length).toBe(SSE_MAX_REFRESH_FAILURES + 3);
    });

    it("a successful refresh resets the failure budget", async () => {
      const refreshSession = vi
        .fn()
        .mockResolvedValueOnce(false)
        .mockResolvedValueOnce(false)
        .mockResolvedValueOnce(true) // outage ends: session still valid
        .mockResolvedValue(false);
      const qc = new QueryClient();
      renderHook(() => useWorkspaceEvents("acme"), {
        wrapper: wrapper(qc, { refreshSession }),
      });

      // Two failures, then a success — the streak resets, so the loop
      // survives well past SSE_MAX_REFRESH_FAILURES total failures.
      for (let i = 0; i < 5; i++) {
        const current = instances[instances.length - 1];
        if (current.readyState !== FakeEventSource.CLOSED) hardClose(current);
        await vi.advanceTimersByTimeAsync(SSE_MAX_BACKOFF_MS);
      }
      // fail, fail, success, fail, fail → 5 attempts, still alive after 3
      // TOTAL failures because they were not consecutive.
      expect(refreshSession).toHaveBeenCalledTimes(5);
      expect(instances.length).toBeGreaterThan(SSE_MAX_REFRESH_FAILURES);
    });
  });
});

describe("SSE reconnect policy (pure)", () => {
  it("waits the exponential backoff, capped", () => {
    let s = initialSseReconnectState();
    const delays: number[] = [];
    for (let i = 0; i < 8; i++) {
      const plan = sseClosed(s);
      if (plan.action !== "retry") throw new Error("unexpected stop");
      delays.push(plan.delayMs);
      s = plan.next;
    }
    expect(delays[0]).toBe(SSE_INITIAL_BACKOFF_MS);
    expect(delays[1]).toBe(SSE_INITIAL_BACKOFF_MS * 2);
    expect(Math.max(...delays)).toBe(SSE_MAX_BACKOFF_MS);
    expect(delays.at(-1)).toBe(SSE_MAX_BACKOFF_MS);
  });

  it("stops once the refresh budget is exhausted", () => {
    let s = initialSseReconnectState();
    for (let i = 0; i < SSE_MAX_REFRESH_FAILURES; i++) {
      expect(sseShouldStop(s)).toBe(false);
      s = sseRefreshResult(s, false);
    }
    expect(sseShouldStop(s)).toBe(true);
    expect(sseClosed(s)).toEqual({ action: "stop" });
  });

  it("a refresh success clears the failure streak", () => {
    let s = sseRefreshResult(sseRefreshResult(initialSseReconnectState(), false), false);
    expect(s.refreshFailures).toBe(2);
    s = sseRefreshResult(s, true);
    expect(s.refreshFailures).toBe(0);
    expect(sseShouldStop(s)).toBe(false);
  });

  it("an open connection resets backoff and budget", () => {
    const plan = sseClosed(initialSseReconnectState());
    if (plan.action !== "retry") throw new Error("unexpected stop");
    const degraded = sseRefreshResult(plan.next, false);
    expect(degraded.backoffMs).toBeGreaterThan(SSE_INITIAL_BACKOFF_MS);
    expect(degraded.refreshFailures).toBe(1);
    expect(sseOpened()).toEqual(initialSseReconnectState());
  });
});
