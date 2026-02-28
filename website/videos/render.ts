import { bundle } from "@remotion/bundler";
import { renderMedia, selectComposition } from "@remotion/renderer";
import path from "path";
import fs from "fs";
import { execFileSync } from "child_process";
import { loadAllScripts, expandThemes, resolveScript } from "./src/parse-script";
import type { ResolvedScript, Script } from "./src/schema";

const ROOT = import.meta.dirname ?? __dirname;
const SCRIPTS_DIR = path.join(ROOT, "scripts");
const OUTPUT_DIR = path.join(ROOT, "output");
const ENTRY = path.join(ROOT, "src", "index.tsx");

/**
 * Get video duration in seconds using ffprobe.
 */
function getVideoDuration(filePath: string): number {
  try {
    const result = execFileSync(
      "ffprobe",
      ["-v", "quiet", "-print_format", "json", "-show_format", filePath],
      { encoding: "utf-8" }
    );
    const data = JSON.parse(result);
    return parseFloat(data.format.duration);
  } catch {
    console.warn(`  Could not probe duration for ${filePath}, using 10s default`);
    return 10;
  }
}

/**
 * Resolve all video durations for a script's recording scenes.
 */
function resolveVideoDurations(script: Script): Map<string, number> {
  const durations = new Map<string, number>();

  for (const scene of script.scenes) {
    if (scene.type === "recording" && scene.duration === "auto") {
      const videoPath = path.join(ROOT, "public", "raw", scene.source);
      if (fs.existsSync(videoPath)) {
        const dur = getVideoDuration(videoPath);
        durations.set(scene.source, dur);
      } else {
        console.warn(`  Video not found: ${videoPath}, using 10s default`);
        durations.set(scene.source, 10);
      }
    }
  }

  return durations;
}

async function main() {
  const filterArg = process.argv[2];

  fs.mkdirSync(OUTPUT_DIR, { recursive: true });

  // Load and expand all scripts
  const rawScripts = loadAllScripts(SCRIPTS_DIR);
  const allScripts = rawScripts.flatMap(expandThemes);

  // Filter if a composition ID was provided
  const scripts = filterArg
    ? allScripts.filter((s) => s.video.id === filterArg)
    : allScripts;

  if (scripts.length === 0) {
    const available = allScripts.map((s) => s.video.id).join(", ");
    console.error(
      `No matching composition found for "${filterArg}". Available: ${available}`
    );
    process.exit(1);
  }

  console.log(`Bundling Remotion project...`);
  const bundleLocation = await bundle({
    entryPoint: ENTRY,
    webpackOverride: (config) => ({
      ...config,
      resolve: {
        ...config.resolve,
        fallback: {
          ...config.resolve?.fallback,
          path: false,
          fs: false,
        },
      },
    }),
    onProgress: (() => {
      let last = -1;
      return (pct: number) => {
        const rounded = Math.round(pct * 10) * 10;
        if (rounded > last) {
          last = rounded;
          console.log(`  Bundling: ${rounded}%`);
        }
      };
    })(),
  });
  console.log("  Bundle complete.");

  for (const script of scripts) {
    const id = script.video.id;
    const outputFile = path.join(OUTPUT_DIR, `${id}.mp4`);

    console.log(`\nRendering: ${id}`);

    // Resolve video durations
    const videoDurations = resolveVideoDurations(script);
    const resolved = resolveScript(script, videoDurations);

    console.log(`  Total duration: ${resolved.totalDurationInFrames} frames (${(resolved.totalDurationInFrames / script.video.fps).toFixed(1)}s)`);

    // Select composition from bundle
    const composition = await selectComposition({
      serveUrl: bundleLocation,
      id,
      inputProps: { script: resolved },
    });

    // Override duration with our resolved value
    composition.durationInFrames = resolved.totalDurationInFrames;

    await renderMedia({
      composition,
      serveUrl: bundleLocation,
      codec: "h264",
      outputLocation: outputFile,
      inputProps: { script: resolved },
      onProgress: (() => {
        let last = -1;
        return ({ progress }: { progress: number }) => {
          const rounded = Math.round(progress * 10) * 10;
          if (rounded > last) {
            last = rounded;
            console.log(`  Rendering: ${rounded}%`);
          }
        };
      })(),
    });

    console.log(`  -> ${outputFile}`);
  }

  console.log("\nAll renders complete.");
}

main().catch((err) => {
  console.error("Render failed:", err);
  process.exit(1);
});
