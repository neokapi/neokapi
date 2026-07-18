import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { createElement } from "react";
import {
  planInvalidations,
  createEventInvalidator,
  useWorkspaceEvents,
  LIST_INVALIDATE_DEBOUNCE_MS,
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
    { type: "brand.profile.updated", expectIncludes: ["brand-candidates", "brand-profiles"] },
    { type: "brand.voice.drift", expectIncludes: ["brand-drift"] },
    { type: "connector.sync.completed", expectIncludes: ["connectors", "project"] },
    { type: "flow.completed", expectIncludes: ["flows", "automation-runs"] },
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

  function wrapper(qc: QueryClient) {
    return ({ children }: { children: ReactNode }) =>
      createElement(QueryClientProvider, { client: qc }, children);
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
});
