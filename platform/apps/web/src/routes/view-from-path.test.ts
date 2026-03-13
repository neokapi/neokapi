import { describe, it, expect } from "vite-plus/test";
import { viewFromPath } from "./view-from-path";

describe("viewFromPath", () => {
  it("returns 'translate' for workspace root", () => {
    expect(viewFromPath("/acme", "acme")).toBe("translate");
  });

  it("returns 'translate' for project routes", () => {
    expect(viewFromPath("/acme/project/abc", "acme")).toBe("translate");
    expect(viewFromPath("/acme/project/abc/translate/file.html", "acme")).toBe("translate");
  });

  it("returns 'termbase' for termbase route", () => {
    expect(viewFromPath("/acme/termbase", "acme")).toBe("termbase");
  });

  it("returns 'memory' for memory route", () => {
    expect(viewFromPath("/acme/memory", "acme")).toBe("memory");
  });

  it("returns 'settings' for settings routes", () => {
    expect(viewFromPath("/acme/settings", "acme")).toBe("settings");
    expect(viewFromPath("/acme/settings/members", "acme")).toBe("settings");
    expect(viewFromPath("/acme/settings/providers", "acme")).toBe("settings");
  });

  it("handles workspace slugs with special characters", () => {
    expect(viewFromPath("/my-workspace/termbase", "my-workspace")).toBe("termbase");
    expect(viewFromPath("/my-workspace/memory", "my-workspace")).toBe("memory");
  });

  it("defaults to 'translate' for unknown paths", () => {
    expect(viewFromPath("/acme/unknown", "acme")).toBe("translate");
  });
});
