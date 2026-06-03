import fs from "node:fs";
import path from "node:path";
import { docsVideoDirFor, ensureDir } from "../lib/paths.ts";
import { sh } from "../lib/exec.ts";
import type { DemoManifest } from "../types.ts";
import { outputPathFor, type ThemeMode } from "./render.ts";

const THEMES: ThemeMode[] = ["dark", "light"];

/**
 * VP9/Opus WebM tuned for the docs site: no fixed bitrate (constant-quality),
 * yuv420p so every browser decodes it, small enough to ship in the docs-assets
 * tarball. ThemedVideo picks the light/dark file from the page's color mode.
 */
async function toWebm(mp4: string, webm: string): Promise<void> {
  // Remotion's mp4 is full-range (yuvj420p) tagged bt470bg with unknown
  // primaries/transfer. Chrome's VP9 decoder (esp. the HW path) is strict about
  // that and intermittently fails to render — Safari is lenient. Convert to
  // limited-range and tag the standard HD colorspace (bt709) explicitly, and drop
  // Remotion's leaked mp4 (isom) container metadata, so the WebM is unambiguous.
  const cmd =
    `ffmpeg -y -i ${JSON.stringify(mp4)} ` +
    `-vf "scale=in_range=full:out_range=tv,format=yuv420p" ` +
    `-c:v libvpx-vp9 -b:v 0 -crf 34 -row-mt 1 -g 240 ` +
    `-colorspace bt709 -color_primaries bt709 -color_trc bt709 -color_range tv ` +
    `-map_metadata -1 ` +
    `-c:a libopus -b:a 96k ${JSON.stringify(webm)}`;
  const r = await sh(cmd, { timeoutMs: 600_000 });
  if (r.code !== 0) throw new Error(`ffmpeg failed (${r.code}) for ${path.basename(mp4)}:\n${r.stderr.slice(-500)}`);
}

// Grab a content frame for the <video> poster. The first frame is blank (the
// title card before it springs in), so it would be invisible once the video is
// theme-matched to the page. 2.5s lands on the settled title card.
const POSTER_AT_SEC = 2.5;
async function toPoster(mp4: string, jpg: string): Promise<void> {
  const cmd = `ffmpeg -y -ss ${POSTER_AT_SEC} -i ${JSON.stringify(mp4)} -vframes 1 -q:v 3 ${JSON.stringify(jpg)}`;
  const r = await sh(cmd, { timeoutMs: 60_000 });
  if (r.code !== 0) throw new Error(`ffmpeg poster failed (${r.code}) for ${path.basename(mp4)}:\n${r.stderr.slice(-500)}`);
}

export interface PublishOptions {
  /** Override the docs video directory (default: routed by brand/target via docsVideoDirFor). */
  docsDir?: string;
}

/**
 * Convert a published demo's rendered mp4s → `<publishAs>-<theme>.webm` in the docs
 * video dir. Demos without `publishAs` are previews only and are skipped.
 */
export async function publishDemo(m: DemoManifest, opts: PublishOptions = {}): Promise<string[]> {
  if (!m.publishAs) {
    console.log(`  · ${m.id}: no publishAs (preview only) — skipping`);
    return [];
  }
  const docsDir = opts.docsDir ?? docsVideoDirFor(m);
  ensureDir(docsDir);
  const written: string[] = [];
  for (const themeMode of THEMES) {
    const mp4 = outputPathFor(m.id, themeMode);
    if (!fs.existsSync(mp4)) {
      console.warn(`  ! ${m.id} (${themeMode}): missing ${path.basename(mp4)} — render it first (--theme=both)`);
      continue;
    }
    const webm = path.join(docsDir, `${m.publishAs}-${themeMode}.webm`);
    const poster = path.join(docsDir, `${m.publishAs}-${themeMode}.jpg`);
    console.log(`  · ${m.id} (${themeMode}) → ${webm}`);
    await toWebm(mp4, webm);
    await toPoster(mp4, poster);
    written.push(webm, poster);
  }
  return written;
}
