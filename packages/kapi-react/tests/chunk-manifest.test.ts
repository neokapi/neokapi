import { describe, it, expect } from "vitest";
import { buildChunkManifest, type BundleLike } from "../src/plugin/chunk-manifest.ts";
import { transform } from "../src/plugin/transform.ts";

describe("buildChunkManifest", () => {
  it("unions hashes from modules in each chunk", () => {
    const hashesByFile = new Map<string, Set<string>>([
      ["/src/App.tsx", new Set(["h1", "h2"])],
      ["/src/SettingsPage.tsx", new Set(["h3"])],
      ["/src/shared.ts", new Set(["h4"])],
    ]);
    const bundle: BundleLike = {
      "assets/index-a.js": {
        type: "chunk",
        name: "index",
        modules: {
          "/src/App.tsx": {},
          "/src/shared.ts": {},
        },
      },
      "assets/SettingsPage-b.js": {
        type: "chunk",
        name: "SettingsPage",
        modules: {
          "/src/SettingsPage.tsx": {},
          "/src/shared.ts": {},
        },
      },
    };

    const manifest = buildChunkManifest(bundle, hashesByFile);
    expect(manifest.version).toBe(1);
    expect(manifest.chunks).toEqual({
      index: ["h1", "h2", "h4"],
      SettingsPage: ["h3", "h4"],
    });
  });

  it("skips assets and chunks with no translatable modules", () => {
    const hashesByFile = new Map<string, Set<string>>([["/src/App.tsx", new Set(["h1"])]]);
    const bundle: BundleLike = {
      "assets/index-a.js": {
        type: "chunk",
        name: "index",
        modules: { "/src/App.tsx": {} },
      },
      "assets/vendor-b.js": {
        type: "chunk",
        name: "vendor",
        modules: { "/node_modules/react/index.js": {} },
      },
      "assets/logo.png": { type: "asset" },
    };

    const manifest = buildChunkManifest(bundle, hashesByFile);
    expect(Object.keys(manifest.chunks)).toEqual(["index"]);
  });

  it("hashes are sorted for reproducible output", () => {
    const hashesByFile = new Map<string, Set<string>>([
      ["/src/A.tsx", new Set(["zzz", "aaa", "mmm"])],
    ]);
    const bundle: BundleLike = {
      "assets/index-a.js": {
        type: "chunk",
        name: "index",
        modules: { "/src/A.tsx": {} },
      },
    };
    const manifest = buildChunkManifest(bundle, hashesByFile);
    expect(manifest.chunks.index).toEqual(["aaa", "mmm", "zzz"]);
  });

  it("merges two chunks sharing the same name", () => {
    const hashesByFile = new Map<string, Set<string>>([
      ["/src/a.tsx", new Set(["h1"])],
      ["/src/b.tsx", new Set(["h2", "h1"])],
    ]);
    const bundle: BundleLike = {
      "chunk-a.js": {
        type: "chunk",
        name: "shared",
        modules: { "/src/a.tsx": {} },
      },
      "chunk-b.js": {
        type: "chunk",
        name: "shared",
        modules: { "/src/b.tsx": {} },
      },
    };
    const manifest = buildChunkManifest(bundle, hashesByFile);
    expect(manifest.chunks).toEqual({ shared: ["h1", "h2"] });
  });

  it("produces empty chunks when no transforms ran", () => {
    const bundle: BundleLike = {
      "assets/index.js": {
        type: "chunk",
        name: "index",
        modules: { "/src/App.tsx": {} },
      },
    };
    const manifest = buildChunkManifest(bundle, new Map());
    expect(manifest).toEqual({ version: 1, chunks: {} });
  });
});

describe("transform returns hashes in runtime mode", () => {
  it("collects hashes from JSX text", () => {
    const result = transform("<h1>Hello</h1>", "Test.tsx", { mode: "runtime" });
    expect(result).not.toBeNull();
    expect(result!.hashes.length).toBe(1);
  });

  it("collects multiple distinct hashes", () => {
    const src = `
      <div>
        <h1>First</h1>
        <h2>Second</h2>
        <p>Third</p>
      </div>
    `;
    const result = transform(src, "Test.tsx", { mode: "runtime" });
    expect(result).not.toBeNull();
    expect(new Set(result!.hashes).size).toBe(3);
  });

  it("returns empty hashes for source with no translatable content", () => {
    const result = transform("<code>x = 1</code>", "Test.tsx", { mode: "runtime" });
    expect(result).toBeNull();
  });

  it("inline mode populates no hashes (baked strings skip runtime)", () => {
    const result = transform("<h1>Hello</h1>", "Test.tsx", {
      mode: "inline",
      locale: "de",
      strict: false,
    });
    expect(result).not.toBeNull();
    expect(result!.hashes).toEqual([]);
  });
});
