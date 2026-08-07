import { describe, it, expect } from "vitest";
import { readCdnConfig, cdnEnabled, cdnHref } from "./cdn";

describe("readCdnConfig", () => {
  it("defaults to a disabled, unversioned config when nothing is set", () => {
    const cfg = readCdnConfig({});
    expect(cfg).toEqual({ base: "", sitePrefix: "", version: "dev", modelsVersion: "v1" });
    expect(cdnEnabled(cfg)).toBe(false);
  });

  it("strips trailing slashes from the base and reads the site fields", () => {
    const cfg = readCdnConfig({
      customFields: {
        cdnBaseUrl: "https://cdn.bowrain.cloud///",
        cdnSitePrefix: "kapi",
        cdnWasmVersion: "abc123",
        cdnModelsVersion: "v3",
      },
    });
    expect(cfg).toEqual({
      base: "https://cdn.bowrain.cloud",
      sitePrefix: "kapi",
      version: "abc123",
      modelsVersion: "v3",
    });
    expect(cdnEnabled(cfg)).toBe(true);
  });

  it("ignores non-string custom fields", () => {
    const cfg = readCdnConfig({ customFields: { cdnBaseUrl: 42, cdnSitePrefix: null } });
    expect(cfg.base).toBe("");
    expect(cfg.sitePrefix).toBe("");
  });
});

describe("cdnHref", () => {
  const cfg = { base: "https://cdn.x", sitePrefix: "kapi", version: "dev", modelsVersion: "v1" };

  it("joins the origin, per-site scope, and a rooted path", () => {
    expect(cdnHref(cfg, "/video/a.webm")).toBe("https://cdn.x/kapi/video/a.webm");
  });

  it("adds a leading slash to a relative path", () => {
    expect(cdnHref(cfg, "video/a.webm")).toBe("https://cdn.x/kapi/video/a.webm");
  });

  it("omits the scope segment when no site prefix is set", () => {
    expect(cdnHref({ ...cfg, sitePrefix: "" }, "/a.webm")).toBe("https://cdn.x/a.webm");
  });
});
