import { describe, it, expect } from "vite-plus/test";
import { projectViewFromPath, viewFromPath } from "./view-from-path";

describe("viewFromPath", () => {
  it("returns 'translate' for workspace root", () => {
    expect(viewFromPath("/acme", "acme")).toBe("translate");
  });

  it("returns 'translate' for project routes", () => {
    expect(viewFromPath("/acme/p/abc/s/main", "acme")).toBe("translate");
    expect(viewFromPath("/acme/p/abc/s/main/file.html/translate", "acme")).toBe("translate");
  });

  it("returns 'terms' for terms route", () => {
    expect(viewFromPath("/acme/terms", "acme")).toBe("terms");
  });

  it("returns 'memory' for the legacy top-level memory route", () => {
    expect(viewFromPath("/acme/memory", "acme")).toBe("memory");
  });

  it("returns 'context' for context hub routes", () => {
    expect(viewFromPath("/acme/context", "acme")).toBe("context");
    expect(viewFromPath("/acme/context/profiles", "acme")).toBe("context");
    expect(viewFromPath("/acme/context/profiles/channel~docs", "acme")).toBe("context");
    expect(viewFromPath("/acme/context/concepts", "acme")).toBe("context");
    expect(viewFromPath("/acme/context/concepts/c-1", "acme")).toBe("context");
    expect(viewFromPath("/acme/context/voice", "acme")).toBe("context");
    expect(viewFromPath("/acme/context/changes", "acme")).toBe("context");
    expect(viewFromPath("/acme/context/activity", "acme")).toBe("context");
  });

  it("returns 'context' for content memory inside the hub", () => {
    // Content memory is a Context sub-nav section, so the rail highlights
    // Context; the sub-nav picks out "memory".
    expect(viewFromPath("/acme/context/memory", "acme")).toBe("context");
  });

  it("returns 'insights' for the context dashboard", () => {
    // The overview reads the graph back to you, so it files under Insights
    // even though it lives on a /context URL.
    expect(viewFromPath("/acme/context/dashboard", "acme")).toBe("insights");
  });

  it("returns 'insights' for locale demand route", () => {
    // Locale demand moved into Insights: demand by market is evidence for a
    // coordinate decision, not a standalone report.
    expect(viewFromPath("/acme/locale-demand", "acme")).toBe("insights");
  });

  it("returns 'settings' for settings routes", () => {
    expect(viewFromPath("/acme/settings", "acme")).toBe("settings");
    expect(viewFromPath("/acme/settings/members", "acme")).toBe("settings");
    expect(viewFromPath("/acme/settings/providers", "acme")).toBe("settings");
  });

  it("handles workspace slugs with special characters", () => {
    expect(viewFromPath("/my-workspace/terms", "my-workspace")).toBe("terms");
    expect(viewFromPath("/my-workspace/memory", "my-workspace")).toBe("memory");
  });

  it("defaults to 'translate' for unknown paths", () => {
    expect(viewFromPath("/acme/unknown", "acme")).toBe("translate");
  });
});

describe("projectViewFromPath", () => {
  it("lights the Map on the Map and in the Reader it opens", () => {
    expect(projectViewFromPath("/acme/p/abc/s/main/map", "acme")).toBe("map");
    expect(projectViewFromPath("/acme/p/abc/s/main/read/docs/a.md", "acme")).toBe("map");
  });

  it("lights Rules on Rules", () => {
    expect(projectViewFromPath("/acme/p/abc/s/main/rules", "acme")).toBe("rules");
  });

  it("lights the Overview on the overview and in a per-file editor", () => {
    expect(projectViewFromPath("/acme/p/abc/s/main/overview", "acme")).toBe("dashboard");
    expect(projectViewFromPath("/acme/p/abc/s/main/translate/docs/a.md", "acme")).toBe("dashboard");
  });

  it("names each other section by its segment", () => {
    for (const s of ["source", "automations", "runs", "connectors"]) {
      expect(projectViewFromPath(`/acme/p/abc/s/main/${s}`, "acme")).toBe(s);
    }
  });

  it("answers undefined outside a project", () => {
    expect(projectViewFromPath("/acme/context", "acme")).toBeUndefined();
    expect(projectViewFromPath("/acme/p/abc", "acme")).toBeUndefined();
  });
});
