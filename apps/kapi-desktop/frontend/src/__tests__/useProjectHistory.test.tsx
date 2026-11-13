import { afterEach, beforeEach, describe, expect, it, vi } from "vite-plus/test";
import { act, renderHook } from "./testUtils";

import { useProjectHistory } from "../hooks/useProjectHistory";
import type { KapiProject } from "../types/api";

const base: KapiProject = { version: "v1", name: "KapiMart" };

function renderHistory(tabId: string | null = "tab-1") {
  return renderHook(({ tab }) => useProjectHistory(base, tab), {
    initialProps: { tab: tabId },
  });
}

describe("project undo history", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("starts clean with nothing to undo", () => {
    const { result } = renderHistory();
    expect(result.current.project).toEqual(base);
    expect(result.current.isDirty).toBe(false);
    expect(result.current.canUndo).toBe(false);
    expect(result.current.canRedo).toBe(false);
  });

  it("commits a burst of edits as one undo step", () => {
    const { result } = renderHistory();
    act(() => result.current.set({ ...base, name: "K" }));
    act(() => result.current.set({ ...base, name: "Ka" }));
    act(() => vi.advanceTimersByTime(500));

    expect(result.current.project.name).toBe("Ka");
    expect(result.current.isDirty).toBe(true);
    expect(result.current.canUndo).toBe(true);

    act(() => result.current.undo());
    expect(result.current.project.name).toBe("KapiMart");
    expect(result.current.isDirty).toBe(false);
    expect(result.current.canUndo).toBe(false);
    expect(result.current.canRedo).toBe(true);

    act(() => result.current.redo());
    expect(result.current.project.name).toBe("Ka");
    expect(result.current.canRedo).toBe(false);
  });

  it("undoes a pending edit without waiting for the pause", () => {
    const { result } = renderHistory();
    act(() => result.current.set({ ...base, name: "Pending" }));
    act(() => result.current.undo());
    expect(result.current.project.name).toBe("KapiMart");
  });

  it("clears the dirty flag on save", () => {
    const { result, rerender } = renderHistory();
    act(() => result.current.set({ ...base, name: "Saved" }));
    expect(result.current.isDirty).toBe(true);
    act(() => result.current.markSaved());
    // markSaved records the snapshot in a ref; the flag reads it on the next render.
    rerender({ tab: "tab-1" });
    expect(result.current.isDirty).toBe(false);
  });

  it("replaces the project without an undo step", () => {
    const { result } = renderHistory();
    act(() => result.current.set({ ...base, name: "Edited" }));
    act(() => vi.advanceTimersByTime(500));
    act(() => result.current.replace({ ...base, preset: "web" }));

    expect(result.current.project.preset).toBe("web");
    expect(result.current.isDirty).toBe(false);
    expect(result.current.canUndo).toBe(false);
  });

  it("keeps each tab's history when switching tabs", () => {
    const { result, rerender } = renderHistory("tab-1");
    act(() => result.current.set({ ...base, name: "One" }));
    act(() => vi.advanceTimersByTime(500));

    rerender({ tab: "tab-2" });
    expect(result.current.project.name).toBe("KapiMart");
    expect(result.current.canUndo).toBe(false);

    rerender({ tab: "tab-1" });
    expect(result.current.project.name).toBe("One");
    expect(result.current.canUndo).toBe(true);
  });
});
