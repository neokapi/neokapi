import fs from "node:fs";
import path from "node:path";
import { execFileSync } from "node:child_process";
import { bundle } from "@remotion/bundler";
import { renderMedia, selectComposition } from "@remotion/renderer";
import { HARNESS_ROOT, OUT_DIR, PUBLIC_DIR, ensureDir, publicDemoDir } from "../lib/paths.ts";

let cachedServeUrl: string | null = null;

let cachedStamp: string | null = null;

/** Provenance stamp burned into every frame: `<version> · <short-sha>[+] · <UTC>`,
 *  so a published video can be traced back to the commit + render time. */
function buildStamp(): string {
  if (cachedStamp !== null) return cachedStamp;
  const git = (...args: string[]): string => {
    try {
      return execFileSync("git", args, { cwd: HARNESS_ROOT, stdio: ["ignore", "pipe", "ignore"] })
        .toString()
        .trim();
    } catch {
      return "";
    }
  };
  const version = git("describe", "--tags", "--abbrev=0") || "dev";
  const sha = git("rev-parse", "--short", "HEAD") || "nogit";
  const dirty = git("status", "--porcelain") ? "+" : "";
  const utc = new Date().toISOString().slice(0, 16).replace("T", " ") + " UTC";
  cachedStamp = `${version} · ${sha}${dirty} · ${utc}`;
  return cachedStamp;
}

/** Bundle the Remotion entry once and reuse across demos. */
async function getServeUrl(): Promise<string> {
  if (cachedServeUrl) return cachedServeUrl;
  console.log("  · bundling Remotion project …");
  cachedServeUrl = await bundle({
    entryPoint: path.join(HARNESS_ROOT, "src", "remotion", "index.ts"),
    publicDir: PUBLIC_DIR,
    onProgress: () => {},
  });
  return cachedServeUrl;
}

export type ThemeMode = "light" | "dark";

export interface RenderOptions {
  force?: boolean;
  quality?: "draft" | "final";
  /** Palette to render with. "dark" → out/<id>.mp4; "light" → out/<id>-light.mp4. */
  themeMode?: ThemeMode;
}

/** Output path for a demo in a given theme (dark is the unsuffixed default). */
export function outputPathFor(id: string, themeMode: ThemeMode): string {
  return path.join(OUT_DIR, themeMode === "light" ? `${id}-light.mp4` : `${id}.mp4`);
}

/** Render a single demo composition → out/<id>.mp4 (dark) or out/<id>-light.mp4. */
export async function renderDemo(id: string, opts: RenderOptions = {}): Promise<string | null> {
  const themeMode: ThemeMode = opts.themeMode ?? "dark";
  const capJson = path.join(publicDemoDir(id), "capture.json");
  if (!fs.existsSync(capJson)) {
    console.warn(`  ! no capture for ${id} — skipping render`);
    return null;
  }
  ensureDir(OUT_DIR);
  const output = outputPathFor(id, themeMode);
  if (!opts.force && fs.existsSync(output)) {
    console.log(`  · video exists for ${id} (${themeMode}) (use --force to re-render)`);
    return output;
  }

  const inputProps = { id, themeMode, stamp: buildStamp() };
  const serveUrl = await getServeUrl();
  const composition = await selectComposition({ serveUrl, id, inputProps });
  console.log(`  · rendering ${id} (${themeMode}): ${composition.durationInFrames} frames (${(composition.durationInFrames / composition.fps).toFixed(1)}s)`);

  let lastPct = -1;
  await renderMedia({
    serveUrl,
    composition,
    codec: "h264",
    outputLocation: output,
    inputProps,
    crf: opts.quality === "draft" ? 28 : 18,
    jpegQuality: opts.quality === "draft" ? 70 : 90,
    concurrency: null,
    onProgress: ({ progress }) => {
      const pct = Math.floor(progress * 100);
      if (pct >= lastPct + 10) {
        lastPct = pct;
        process.stdout.write(`\r    render ${id} (${themeMode}): ${pct}%   `);
      }
    },
  });
  process.stdout.write("\n");
  console.log(`  ✓ rendered ${output}`);
  return output;
}
