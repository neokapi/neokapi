import fs from "node:fs";
import path from "node:path";
import { execFileSync } from "node:child_process";
import { bundle } from "@remotion/bundler";
import { renderMedia, selectComposition } from "@remotion/renderer";
import { HARNESS_ROOT, OUT_DIR, PUBLIC_DIR, ensureDir, publicDemoDir } from "../lib/paths.ts";
import { isDefaultLocale, localeSuffix, resolveLocale } from "../lib/locale.ts";

let cachedServeUrl: string | null = null;

let cachedStamp: string | null = null;

/** Provenance stamp burned into every frame: `<version> · <short-sha> · <UTC>`,
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
  const utc = new Date().toISOString().slice(0, 16).replace("T", " ") + " UTC";
  cachedStamp = `${version} · ${sha} · ${utc}`;
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
  /** Narration locale (default "en"). Non-default locales render from
   *  narration-<locale>.json into out/<id>-<locale>[-light].mp4. */
  locale?: string;
}

/** Output path for a demo in a given theme + locale (dark/en are the unsuffixed defaults). */
export function outputPathFor(id: string, themeMode: ThemeMode, locale?: string): string {
  const loc = localeSuffix(resolveLocale(locale));
  return path.join(OUT_DIR, themeMode === "light" ? `${id}${loc}-light.mp4` : `${id}${loc}.mp4`);
}

/** Render a single demo composition → out/<id>[-<locale>][-light].mp4. */
export async function renderDemo(id: string, opts: RenderOptions = {}): Promise<string | null> {
  const themeMode: ThemeMode = opts.themeMode ?? "dark";
  const locale = resolveLocale(opts.locale);
  const capJson = path.join(publicDemoDir(id), "capture.json");
  if (!fs.existsSync(capJson)) {
    console.warn(`  ! no capture for ${id} — skipping render`);
    return null;
  }
  // A non-default locale needs its narration synthesized first — without this
  // check the composition would silently fall back to the English track.
  if (!isDefaultLocale(locale) && !fs.existsSync(path.join(publicDemoDir(id), `narration${localeSuffix(locale)}.json`))) {
    console.warn(`  ! no ${locale} narration for ${id} — run the narrate stage with --locale=${locale} first; skipping render`);
    return null;
  }
  ensureDir(OUT_DIR);
  const output = outputPathFor(id, themeMode, locale);
  const variant = isDefaultLocale(locale) ? themeMode : `${themeMode}, ${locale}`;
  if (!opts.force && fs.existsSync(output)) {
    console.log(`  · video exists for ${id} (${variant}) (use --force to re-render)`);
    return output;
  }

  // `locale` rides inputProps so Root.tsx's calculateMetadata loads
  // narration-<locale>.json (falling back to the English narration.json).
  const inputProps = { id, themeMode, locale, stamp: buildStamp() };
  const serveUrl = await getServeUrl();
  const composition = await selectComposition({ serveUrl, id, inputProps });
  console.log(`  · rendering ${id} (${variant}): ${composition.durationInFrames} frames (${(composition.durationInFrames / composition.fps).toFixed(1)}s)`);

  // The headless render browser occasionally crashes mid-render ("Target closed")
  // or a frame's OffthreadVideo seek stalls — both transient. Retry the whole
  // render once before failing. Concurrency is capped (cores−2) so a fleet of
  // video-decoding render tabs doesn't starve/crash each other.
  const attempts = 2;
  for (let attempt = 1; attempt <= attempts; attempt++) {
    let lastPct = -1;
    try {
      await renderMedia({
        serveUrl,
        composition,
        codec: "h264",
        outputLocation: output,
        inputProps,
        crf: opts.quality === "draft" ? 28 : 18,
        jpegQuality: opts.quality === "draft" ? 70 : 90,
        // Lower concurrency = fewer render tabs hammering Remotion's single
        // video-proxy when seeking the screencast. Long demos (explorer) need a
        // smaller value; tune via HARNESS_RENDER_CONCURRENCY (default 4).
        concurrency: Math.max(1, Number(process.env.HARNESS_RENDER_CONCURRENCY) || 4),
        // Desktop scenes embed a screencast .webm and seek into it per beat;
        // decoding a seek (and loading poster <Img>/fonts) can exceed Remotion's
        // 30s default delayRender budget under heavy machine load, intermittently
        // failing a frame. Generous headroom so a slow-under-load asset finishes
        // rather than timing out (tune via HARNESS_RENDER_TIMEOUT_MS).
        timeoutInMilliseconds: Math.max(180_000, Number(process.env.HARNESS_RENDER_TIMEOUT_MS) || 600_000),
        onProgress: ({ progress }) => {
          const pct = Math.floor(progress * 100);
          if (pct >= lastPct + 10) {
            lastPct = pct;
            process.stdout.write(`\r    render ${id} (${variant}): ${pct}%   `);
          }
        },
      });
      process.stdout.write("\n");
      console.log(`  ✓ rendered ${output}`);
      return output;
    } catch (e) {
      process.stdout.write("\n");
      if (attempt === attempts) throw e;
      console.warn(`  ! render ${id} (${variant}) attempt ${attempt} failed (${(e as Error)?.message?.slice(0, 80)}); retrying …`);
    }
  }
  return output;
}
