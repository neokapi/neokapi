import { afterEach, beforeEach, describe, expect, it, vi } from "vite-plus/test";
import type { ReactNode } from "react";
import { act, renderHook, waitFor } from "./testUtils";

import { api } from "../hooks/useApi";
import { landingView, useTabManager } from "../hooks/useTabManager";
import { ErrorProvider } from "../components/ErrorBanner";

const wrapper = ({ children }: { children: ReactNode }) => (
  <ErrorProvider>{children}</ErrorProvider>
);

const TAB = { id: "tab-1", name: "KapiMart", path: "/fakehome/src/kapimart/kapi.yaml" };

function stubBackend({ empty = false, preset = "" } = {}) {
  vi.spyOn(api, "getAppMode").mockResolvedValue("projects");
  vi.spyOn(api, "getSessionState").mockResolvedValue({
    mode: "projects",
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

  it("sends a project that must be set up first to the page that sets it up", () => {
    expect(landingView(true)).toBe("project-home");
    expect(landingView(false, "i18next")).toBe("project-home");
    expect(landingView(false)).toBe("map");
  });
});
