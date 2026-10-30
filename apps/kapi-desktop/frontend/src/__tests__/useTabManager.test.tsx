import { afterEach, beforeEach, describe, expect, it, vi } from "vite-plus/test";
import type { ReactNode } from "react";
import { act, renderHook, waitFor } from "./testUtils";
import { Events } from "@wailsio/runtime";

import { api } from "../hooks/useApi";
import { landingView, useTabManager } from "../hooks/useTabManager";
import { ErrorProvider } from "../components/ErrorBanner";

const wrapper = ({ children }: { children: ReactNode }) => (
  <ErrorProvider>{children}</ErrorProvider>
);

const TAB = { id: "tab-1", name: "KapiMart", path: "/fakehome/src/kapimart/kapi.yaml" };

function stubBackend({ empty = false, preset = "" } = {}) {
  vi.spyOn(api, "getSessionState").mockResolvedValue({
    lastOpenProjects: [],
    activeProject: "",
  });
  vi.spyOn(api, "saveSessionState").mockResolvedValue(null);
  vi.spyOn(api, "openProject").mockResolvedValue(TAB);
  vi.spyOn(api, "getProject").mockResolvedValue({ version: "v1", name: "KapiMart" });
  vi.spyOn(api, "isEmptyProject").mockResolvedValue(empty);
  vi.spyOn(api, "detectPreset").mockResolvedValue(preset);
  vi.spyOn(api, "checkProjectPlugins").mockResolvedValue({ satisfied: true, issues: [] });
}

describe("where a project tab lands", () => {
  beforeEach(() => stubBackend());
  afterEach(() => vi.restoreAllMocks());

  it("opens a project on its Map", async () => {
    const { result } = renderHook(() => useTabManager(), { wrapper });
    await act(() => result.current.openRecent(TAB.path));
    await waitFor(() => expect(result.current.activeTab?.view).toBe("map"));
    expect(result.current.effectiveView).toBe("map");
  });

  it("opens a project on the view a caller names", async () => {
    const { result } = renderHook(() => useTabManager(), { wrapper });
    await act(() => result.current.openRecent(TAB.path, "context:rules"));
    await waitFor(() => expect(result.current.activeTab?.view).toBe("context:rules"));
  });

  it("moves a tab that is already open to the view a caller names", async () => {
    const { result } = renderHook(() => useTabManager(), { wrapper });
    await act(() => result.current.openRecent(TAB.path));
    act(() => result.current.navigate("checks"));
    await act(() => result.current.openRecent(TAB.path, "context:rules"));
    expect(result.current.tabs).toHaveLength(1);
    expect(result.current.activeTab?.view).toBe("context:rules");
  });

  it("brings an open tab forward on one section", async () => {
    const { result } = renderHook(() => useTabManager(), { wrapper });
    await act(() => result.current.openRecent(TAB.path));
    act(() => result.current.navigate("home"));
    expect(result.current.effectiveView).toBe("home");
    act(() => result.current.openTabOn(TAB.id, "context:agent"));
    expect(result.current.effectiveView).toBe("context:agent");
  });

  it("lands every project on its Map, one waiting to be set up too", () => {
    expect(landingView()).toBe("map");
  });
});

describe("a folder opened without a recipe", () => {
  beforeEach(() => stubBackend());
  afterEach(() => vi.restoreAllMocks());

  it("marks the tab once the backend writes its recipe", async () => {
    vi.mocked(Events.On).mockClear();
    vi.spyOn(api, "openProject").mockResolvedValue({ ...TAB, recipe_written: false });
    const { result } = renderHook(() => useTabManager(), { wrapper });
    await act(() => result.current.openRecent("/fakehome/src/kapimart"));
    expect(result.current.activeTab?.info.recipe_written).toBe(false);

    const call = vi.mocked(Events.On).mock.calls.find((c) => c[0] === "project:recipe-written");
    expect(call).toBeDefined();
    act(() => (call![1] as (e: { data: unknown }) => void)({ data: { tabID: TAB.id } }));
    expect(result.current.activeTab?.info.recipe_written).toBe(true);
  });
});

describe("restoring the remembered session", () => {
  beforeEach(() => stubBackend());
  afterEach(() => vi.restoreAllMocks());

  it("stays restoring until the remembered tab is back", async () => {
    vi.spyOn(api, "getSessionState").mockResolvedValue({
      lastOpenProjects: [TAB.path],
      activeProject: TAB.path,
    });
    const seen: Array<{ restoring: boolean; tabs: number }> = [];
    const { result } = renderHook(
      () => {
        const tm = useTabManager();
        seen.push({ restoring: tm.restoring, tabs: tm.tabs.length });
        return tm;
      },
      { wrapper },
    );
    expect(result.current.restoring).toBe(true);
    await waitFor(() => expect(result.current.restoring).toBe(false));
    expect(result.current.tabs).toHaveLength(1);
    expect(seen.some((s) => !s.restoring && s.tabs === 0)).toBe(false);
  });

  it("ends restoring when there is nothing to reopen", async () => {
    const { result } = renderHook(() => useTabManager(), { wrapper });
    await waitFor(() => expect(result.current.restoring).toBe(false));
    expect(result.current.tabs).toHaveLength(0);
  });
});

describe("a folder handed to the app as it starts", () => {
  const OUTSIDE = { id: "tab-2", name: "site", path: "/fakehome/src/site/kapi.yaml" };
  beforeEach(() => stubBackend());
  afterEach(() => vi.restoreAllMocks());

  it("is shown in front of the remembered session", async () => {
    vi.spyOn(api, "getSessionState").mockResolvedValue({
      lastOpenProjects: [TAB.path],
      activeProject: TAB.path,
    });
    vi.spyOn(api, "listTabs").mockResolvedValue([TAB, OUTSIDE]);
    const { result } = renderHook(() => useTabManager(), { wrapper });
    await waitFor(() => expect(result.current.restoring).toBe(false));
    expect(result.current.tabs.map((t) => t.info.id)).toEqual([TAB.id, OUTSIDE.id]);
    expect(result.current.activeTab?.info.id).toBe(OUTSIDE.id);
    expect(result.current.effectiveView).toBe("map");
  });

  it("is shown in place of the home when nothing was remembered", async () => {
    vi.spyOn(api, "listTabs").mockResolvedValue([OUTSIDE]);
    const { result } = renderHook(() => useTabManager(), { wrapper });
    await waitFor(() => expect(result.current.restoring).toBe(false));
    expect(result.current.tabs.map((t) => t.info.id)).toEqual([OUTSIDE.id]);
    expect(result.current.activeTab?.info.id).toBe(OUTSIDE.id);
    expect(result.current.effectiveView).toBe("map");
  });

  it("leaves the remembered session as it was when nothing came from outside", async () => {
    vi.spyOn(api, "getSessionState").mockResolvedValue({
      lastOpenProjects: [TAB.path],
      activeProject: TAB.path,
    });
    vi.spyOn(api, "listTabs").mockResolvedValue([TAB]);
    const { result } = renderHook(() => useTabManager(), { wrapper });
    await waitFor(() => expect(result.current.restoring).toBe(false));
    expect(result.current.tabs).toHaveLength(1);
    expect(result.current.activeTab?.info.id).toBe(TAB.id);
  });
});
