import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { createElement } from "react";
import { invalidateForEvent, useWorkspaceEvents } from "./useWorkspaceEvents";

describe("invalidateForEvent → query-key mapping", () => {
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
    it(`invalidates ${tc.expectIncludes.join("+")} for "${tc.type}"`, () => {
      const { qc, keys } = capture();
      invalidateForEvent(qc, "acme", { type: tc.type });
      const roots = keys.map((k) => k[0]);
      for (const expected of tc.expectIncludes) {
        expect(roots).toContain(expected);
      }
      // Every invalidated key is workspace-scoped.
      for (const k of keys) {
        expect(k[1]).toBe("acme");
      }
    });
  }
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

    const invalidatedRoots = spy.mock.calls.map(
      (c) => (c[0] as { queryKey: unknown[] }).queryKey[0],
    );
    expect(invalidatedRoots).toContain("project");
    expect(invalidatedRoots).toContain("translationDashboard");
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
