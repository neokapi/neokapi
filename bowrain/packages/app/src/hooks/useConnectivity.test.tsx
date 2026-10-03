import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { act, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import type { FailedChange } from "@neokapi/ui";
import { PlatformProvider, type PlatformAdapter } from "../platform";
import { CONNECTIVITY_POLL_MS, useConnectivity } from "./useConnectivity";

function failed(id: number): FailedChange {
  return {
    id,
    status: "failed",
    operation: "change_set",
    edits: [{ op: "set_content", block: `b${id}`, locale: "fr", text: "Bonjour" }],
    reason: "HTTP 409",
    queued_at: "2026-10-03T12:00:00Z",
  };
}

/** A desktop connectivity seam over an in-memory list of failed changes. */
function desktop(initial: FailedChange[]) {
  let list = initial;
  const conn = {
    state: () => "connected" as const,
    onChange: () => () => {},
    pendingCount: vi.fn(async () => 0),
    failedChanges: vi.fn(async () => list),
    failedChangeIds: vi.fn(async () => list.map((c) => c.id)),
    dismissFailedChange: vi.fn(async (_id: number) => {}),
    dismissFailedChanges: vi.fn(async () => {}),
  };
  const platform: PlatformAdapter = { kind: "desktop", openExternal: () => {}, connectivity: conn };
  const wrapper = ({ children }: { children: ReactNode }) => (
    <PlatformProvider platform={platform}>{children}</PlatformProvider>
  );
  return {
    conn,
    wrapper,
    set(next: FailedChange[]) {
      list = next;
    },
  };
}

async function tick(ms = CONNECTIVITY_POLL_MS) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}

describe("useConnectivity", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("reads the failed list again only when its ids change", async () => {
    const d = desktop([failed(7)]);
    const { result } = renderHook(() => useConnectivity(), { wrapper: d.wrapper });
    await tick(0);
    expect(result.current.failedChanges?.map((c) => c.id)).toEqual([7]);
    const drawn = result.current.failedChanges;

    await tick();
    await tick();
    await tick();
    expect(d.conn.failedChangeIds).toHaveBeenCalledTimes(3);
    expect(d.conn.failedChanges).toHaveBeenCalledTimes(1);
    expect(result.current.failedChanges).toBe(drawn);

    d.set([failed(7), failed(9)]);
    await tick();
    expect(d.conn.failedChanges).toHaveBeenCalledTimes(2);
    expect(result.current.failedChanges?.map((c) => c.id)).toEqual([7, 9]);
  });

  it("reads the list again after a dismissal, whether or not it succeeded", async () => {
    const d = desktop([failed(7), failed(9)]);
    d.conn.dismissFailedChange.mockRejectedValueOnce(new Error("database is locked"));
    const { result } = renderHook(() => useConnectivity(), { wrapper: d.wrapper });
    await tick(0);

    const reads = d.conn.failedChangeIds.mock.calls.length;
    act(() => result.current.dismissFailedChange?.(7));
    await tick(0);
    expect(d.conn.dismissFailedChange).toHaveBeenCalledWith(7);
    expect(d.conn.failedChangeIds.mock.calls.length).toBeGreaterThan(reads);
    expect(result.current.failedChanges?.map((c) => c.id)).toEqual([7, 9]);

    d.set([failed(9)]);
    act(() => result.current.dismissFailedChange?.(7));
    await tick(0);
    expect(result.current.failedChanges?.map((c) => c.id)).toEqual([9]);
  });
});
