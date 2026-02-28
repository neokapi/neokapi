import fs from "fs";
import path from "path";
import YAML from "yaml";
import { ScriptSchema, type Script, type ResolvedScript, type ResolvedScene } from "./schema";

/**
 * Load and validate a YAML script file.
 */
export function loadScript(filePath: string): Script {
  const raw = fs.readFileSync(filePath, "utf-8");
  const parsed = YAML.parse(raw);
  return ScriptSchema.parse(parsed);
}

/**
 * Load all YAML scripts from a directory.
 */
export function loadAllScripts(dir: string): Script[] {
  const files = fs
    .readdirSync(dir)
    .filter((f) => f.endsWith(".yaml") || f.endsWith(".yml"))
    .sort();
  return files.map((f) => loadScript(path.join(dir, f)));
}

/**
 * Expand a script with themes into one script per theme,
 * substituting {theme} in video.id and recording source paths.
 */
export function expandThemes(script: Script): Script[] {
  const themes = script.video.themes;
  if (!themes || themes.length === 0) {
    return [script];
  }

  return themes.map((theme) => ({
    ...script,
    video: {
      ...script.video,
      id: script.video.id.replace(/\{theme\}/g, theme),
      title: script.video.title,
      themes: undefined,
    },
    scenes: script.scenes.map((scene) => {
      if (scene.type === "recording") {
        return {
          ...scene,
          source: scene.source.replace(/\{theme\}/g, theme),
        };
      }
      return scene;
    }),
  }));
}

/**
 * Resolve scene durations. Recording scenes with duration "auto"
 * need the actual video duration, which is provided via videoDurations map.
 * Returns a ResolvedScript with all durations in frames.
 */
export function resolveScript(
  script: Script,
  videoDurations: Map<string, number>
): ResolvedScript {
  const fps = script.video.fps;
  const resolvedScenes: ResolvedScene[] = script.scenes.map((scene) => {
    let durationInFrames: number;

    switch (scene.type) {
      case "title-card":
        durationInFrames = Math.round(scene.duration * fps);
        break;
      case "transition":
        durationInFrames = Math.round(scene.duration * fps);
        break;
      case "recording": {
        if (scene.duration === "auto") {
          const videoDuration = videoDurations.get(scene.source);
          if (videoDuration === undefined) {
            throw new Error(
              `No duration found for video: ${scene.source}. ` +
                `Available: ${[...videoDurations.keys()].join(", ")}`
            );
          }
          let seconds = videoDuration;
          if (scene.trim) {
            seconds -= scene.trim.start;
            if (scene.trim.end < 0) {
              seconds += scene.trim.end;
            } else if (scene.trim.end > 0) {
              seconds = scene.trim.end - scene.trim.start;
            }
          }
          durationInFrames = Math.round(seconds * fps);
        } else {
          durationInFrames = Math.round(scene.duration * fps);
        }
        break;
      }
    }

    return { scene, durationInFrames };
  });

  const totalDurationInFrames = resolvedScenes.reduce(
    (sum, s) => sum + s.durationInFrames,
    0
  );

  return {
    video: script.video,
    branding: script.branding,
    scenes: resolvedScenes,
    totalDurationInFrames,
  };
}
