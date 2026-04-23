import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { runSplit } from "../src/commands/split.ts";
import type { TranslationsManifest } from "../src/plugin/chunk-manifest.ts";

let workDir: string;
let logSpy: ReturnType<typeof vi.spyOn>;

beforeEach(() => {
  workDir = mkdtempSync(join(tmpdir(), "kapi-react-split-"));
  logSpy = vi.spyOn(console, "log").mockImplementation(() => {});
});

afterEach(() => {
  rmSync(workDir, { recursive: true, force: true });
  logSpy.mockRestore();
});

function writeManifest(path: string, manifest: TranslationsManifest) {
  writeFileSync(path, JSON.stringify(manifest, null, 2));
}

describe("kapi-react split", () => {
  it("produces per-locale per-chunk subset files", async () => {
    const manifestPath = join(workDir, "translations-manifest.json");
    const localesDir = join(workDir, "locales");
    const outDir = join(workDir, "out");
    mkdirSync(localesDir);

    writeManifest(manifestPath, {
      version: 1,
      chunks: {
        index: ["h1", "h2"],
        Settings: ["h3"],
      },
    });
    writeFileSync(
      join(localesDir, "de.json"),
      JSON.stringify({ h1: "Hallo", h2: "Welt", h3: "Einstellungen" }),
    );
    writeFileSync(
      join(localesDir, "fr.json"),
      JSON.stringify({ h1: "Bonjour", h2: "Monde", h3: "Paramètres" }),
    );

    await runSplit(["--manifest", manifestPath, "--locales", localesDir, "--out", outDir]);

    const deIndex = JSON.parse(readFileSync(join(outDir, "de/index.json"), "utf8"));
    expect(deIndex).toEqual({ h1: "Hallo", h2: "Welt" });

    const deSettings = JSON.parse(readFileSync(join(outDir, "de/Settings.json"), "utf8"));
    expect(deSettings).toEqual({ h3: "Einstellungen" });

    const frIndex = JSON.parse(readFileSync(join(outDir, "fr/index.json"), "utf8"));
    expect(frIndex).toEqual({ h1: "Bonjour", h2: "Monde" });

    // Manifest is echoed so consumers have a single known path.
    const echoed = JSON.parse(readFileSync(join(outDir, "manifest.json"), "utf8"));
    expect(echoed.chunks.index).toEqual(["h1", "h2"]);
  });

  it("duplicates entries that appear in multiple chunks", async () => {
    const manifestPath = join(workDir, "manifest.json");
    const localesDir = join(workDir, "locales");
    const outDir = join(workDir, "out");
    mkdirSync(localesDir);

    // `shared` hash is used by both chunks — both subsets should include it.
    writeManifest(manifestPath, {
      version: 1,
      chunks: {
        index: ["h1", "shared"],
        Settings: ["h2", "shared"],
      },
    });
    writeFileSync(
      join(localesDir, "de.json"),
      JSON.stringify({ h1: "eins", h2: "zwei", shared: "geteilt" }),
    );

    await runSplit(["--manifest", manifestPath, "--locales", localesDir, "--out", outDir]);

    const deIndex = JSON.parse(readFileSync(join(outDir, "de/index.json"), "utf8"));
    const deSettings = JSON.parse(readFileSync(join(outDir, "de/Settings.json"), "utf8"));
    expect(deIndex.shared).toBe("geteilt");
    expect(deSettings.shared).toBe("geteilt");
  });

  it("omits hashes missing from a locale rather than writing undefined", async () => {
    const manifestPath = join(workDir, "manifest.json");
    const localesDir = join(workDir, "locales");
    const outDir = join(workDir, "out");
    mkdirSync(localesDir);

    writeManifest(manifestPath, {
      version: 1,
      chunks: { index: ["h1", "h2", "h3"] },
    });
    // de has only h1 and h3; h2 is still untranslated.
    writeFileSync(join(localesDir, "de.json"), JSON.stringify({ h1: "Hallo", h3: "Drei" }));

    await runSplit(["--manifest", manifestPath, "--locales", localesDir, "--out", outDir]);

    const deIndex = JSON.parse(readFileSync(join(outDir, "de/index.json"), "utf8"));
    expect(deIndex).toEqual({ h1: "Hallo", h3: "Drei" });
    expect("h2" in deIndex).toBe(false);
  });
});
