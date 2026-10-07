/**
 * One still per scene, at the frame the scene settles on: its last frame
 * before the next scene starts to come in. The check a person looks at before
 * a whole video is rendered.
 *
 *   tsx src/cli/stills.ts <id> [--theme=light|dark|both] [--out=<dir>] [--scale=<n>] [--scene=<id>] [--frames=<n,n>]
 *
 * Writes <out>/<id>-<NN>-<scene>-<theme>.png (default out: out/stills/).
 */
import path from "node:path";
import { bundle } from "@remotion/bundler";
import { renderStill, selectComposition } from "@remotion/renderer";
import type { BeatsFile, DemoCapture, NarrationManifest } from "../types.ts";
import { HARNESS_ROOT, OUT_DIR, PUBLIC_DIR, ensureDir } from "../lib/paths.ts";
import { loadManifest } from "../lib/manifest.ts";
import { writeBeats } from "./beats.ts";
import { computeTiming } from "../remotion/timeline.ts";
import { mergeScenes, transitionFrames } from "../remotion/scene-plan.ts";

async function main() {
  const args = process.argv.slice(2);
  const id = args.find((a) => !a.startsWith("--"));
  if (!id) {
    console.error("usage: tsx src/cli/stills.ts <id> [--theme=light|dark|both] [--out=<dir>] [--scale=<n>] [--scene=<id>] [--frames=<n,n>]");
    process.exit(1);
  }
  const opt = (k: string) => args.find((a) => a.startsWith(`--${k}=`))?.slice(k.length + 3);
  const themeArg = opt("theme") ?? "both";
  const themes = themeArg === "both" ? ["light", "dark"] : [themeArg];
  const out = ensureDir(path.resolve(opt("out") ?? path.join(OUT_DIR, "stills")));
  const scale = Number(opt("scale") ?? "1");
  const only = opt("scene");
  // Particular frames instead of the settled ones, to look at a beat mid-motion.
  const frames = (opt("frames") ?? "").split(",").filter(Boolean).map(Number);

  writeBeats(loadManifest(id));
  const serveUrl = await bundle({ entryPoint: path.join(HARNESS_ROOT, "src", "remotion", "index.ts"), publicDir: PUBLIC_DIR, onProgress: () => {} });
  for (const themeMode of themes) {
    const inputProps = { id, themeMode };
    const composition = await selectComposition({ serveUrl, id, inputProps });
    const props = composition.props as { narration?: NarrationManifest | null; beats?: BeatsFile | null; capture?: DemoCapture | null };
    const narration = props.narration ?? { id, backend: "none", voice: "none", scenes: [] };
    const scenes = mergeScenes(narration, props.beats ?? null);
    const timing = computeTiming(scenes, composition.fps, { transition: transitionFrames, events: props.capture?.events });
    if (frames.length > 0) {
      for (const frame of frames) {
        const file = path.join(out, `${id}-frame${String(frame).padStart(5, "0")}-${themeMode}.png`);
        await renderStill({ serveUrl, composition, frame, output: file, inputProps, scale });
        console.log(`  ✓ ${path.basename(file)}`);
      }
      continue;
    }
    for (const [n, t] of timing.scenes.entries()) {
      if (only && t.id !== only) continue;
      const settled = t.from + t.durationFrames - t.transitionAfter - 2;
      const file = path.join(out, `${id}-${String(n + 1).padStart(2, "0")}-${t.id}-${themeMode}.png`);
      await renderStill({ serveUrl, composition, frame: settled, output: file, inputProps, scale });
      console.log(`  ✓ ${path.basename(file)} (frame ${settled}, ${(settled / composition.fps).toFixed(2)}s)`);
    }
  }
}

main().catch((e) => {
  console.error(e?.stack || e);
  process.exit(1);
});
