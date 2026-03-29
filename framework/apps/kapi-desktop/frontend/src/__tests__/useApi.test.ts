import { describe, it, expect } from "vitest";
import { api, call } from "../hooks/useApi";

describe("useApi", () => {
  it("returns null when not in Wails runtime", async () => {
    const result = await call("NonexistentMethod");
    expect(result).toBeNull();
  });

  it("exposes all project methods", () => {
    expect(api.newProject).toBeDefined();
    expect(api.openProject).toBeDefined();
    expect(api.saveProject).toBeDefined();
    expect(api.saveProjectAs).toBeDefined();
    expect(api.getProject).toBeDefined();
  });

  it("exposes all flow methods", () => {
    expect(api.listFlows).toBeDefined();
    expect(api.getFlow).toBeDefined();
    expect(api.saveFlow).toBeDefined();
    expect(api.deleteFlow).toBeDefined();
  });

  it("exposes all runner methods", () => {
    expect(api.runFlow).toBeDefined();
    expect(api.cancelRun).toBeDefined();
    expect(api.getRunState).toBeDefined();
  });

  it("exposes all tool methods", () => {
    expect(api.listTools).toBeDefined();
    expect(api.getToolSchema).toBeDefined();
    expect(api.listFormats).toBeDefined();
    expect(api.detectFormat).toBeDefined();
  });

  it("exposes all plugin methods", () => {
    expect(api.listPlugins).toBeDefined();
    expect(api.searchPlugins).toBeDefined();
    expect(api.installPlugin).toBeDefined();
    expect(api.removePlugin).toBeDefined();
    expect(api.checkPluginUpdates).toBeDefined();
  });

  it("exposes all credential methods", () => {
    expect(api.listProviders).toBeDefined();
    expect(api.saveProvider).toBeDefined();
    expect(api.deleteProvider).toBeDefined();
    expect(api.testProvider).toBeDefined();
  });

  it("exposes all persistence methods", () => {
    expect(api.listRecentFiles).toBeDefined();
    expect(api.clearRecentFiles).toBeDefined();
    expect(api.getSettings).toBeDefined();
    expect(api.saveSettings).toBeDefined();
    expect(api.getTheme).toBeDefined();
    expect(api.setTheme).toBeDefined();
  });

  it("gracefully handles missing backend", async () => {
    // All methods should return null outside Wails.
    expect(await api.getVersion()).toBeNull();
    expect(await api.listPlugins()).toBeNull();
    expect(await api.listRecentFiles()).toBeNull();
  });
});
