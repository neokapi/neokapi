import { describe, it, expect, beforeEach } from "vite-plus/test";
import { useUIStore } from "./ui-store";

describe("useUIStore", () => {
  beforeEach(() => {
    // Reset store state between tests.
    useUIStore.setState({
      sidebarCollapsed: false,
      lastWorkspaceSlug: null,
    });
  });

  it("defaults sidebarCollapsed to false", () => {
    expect(useUIStore.getState().sidebarCollapsed).toBe(false);
  });

  it("toggles sidebarCollapsed", () => {
    useUIStore.getState().setSidebarCollapsed(true);
    expect(useUIStore.getState().sidebarCollapsed).toBe(true);

    useUIStore.getState().setSidebarCollapsed(false);
    expect(useUIStore.getState().sidebarCollapsed).toBe(false);
  });

  it("defaults lastWorkspaceSlug to null", () => {
    expect(useUIStore.getState().lastWorkspaceSlug).toBeNull();
  });

  it("sets and clears lastWorkspaceSlug", () => {
    useUIStore.getState().setLastWorkspaceSlug("acme");
    expect(useUIStore.getState().lastWorkspaceSlug).toBe("acme");

    useUIStore.getState().setLastWorkspaceSlug(null);
    expect(useUIStore.getState().lastWorkspaceSlug).toBeNull();
  });

  it("persists state to localStorage under 'bowrain-ui' key", () => {
    useUIStore.getState().setSidebarCollapsed(true);
    useUIStore.getState().setLastWorkspaceSlug("test-ws");

    const stored = localStorage.getItem("bowrain-ui");
    expect(stored).toBeTruthy();

    const parsed = JSON.parse(stored!);
    expect(parsed.state.sidebarCollapsed).toBe(true);
    expect(parsed.state.lastWorkspaceSlug).toBe("test-ws");
  });
});
