import { describe, it, expect } from "vitest";
import { api, call } from "../hooks/useApi";

describe("useApi", () => {
  it("returns null when backend is not available", async () => {
    const result = await call("NonexistentMethod");
    expect(result).toBeNull();
  });

  it("exposes all project methods", () => {
    expect(api.newProject).toBeDefined();
    expect(api.openProject).toBeDefined();
    expect(api.openProjectDialog).toBeDefined();
    expect(api.saveProject).toBeDefined();
    expect(api.saveProjectAs).toBeDefined();
    expect(api.saveProjectDialog).toBeDefined();
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

  it("exposes inspect + media methods", () => {
    expect(api.inspectFile).toBeDefined();
    expect(api.inspectFileAnnotated).toBeDefined();
    expect(api.mediaDataURL).toBeDefined();
  });

  it("mediaDataURL returns null without a Wails backend", async () => {
    await expect(api.mediaDataURL("/tmp/x.png")).resolves.toBeNull();
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

  it("gracefully returns null when backend unavailable", async () => {
    // In vitest, the Wails binding import fails silently — all calls return null.
    expect(await api.getVersion()).toBeNull();
    expect(await api.listPlugins()).toBeNull();
    expect(await api.listRecentFiles()).toBeNull();
  });
});
