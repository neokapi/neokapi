import { describe, it, expect } from "vite-plus/test";
import { viewFromPath } from "./view-from-path";

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

  it("returns 'memory' for memory route", () => {
    expect(viewFromPath("/acme/memory", "acme")).toBe("memory");
  });

  it("returns 'locale-demand' for locale demand route", () => {
    expect(viewFromPath("/acme/locale-demand", "acme")).toBe("locale-demand");
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
