import React from "react";
import { Composition, registerRoot } from "remotion";
import { DemoVideo } from "./compositions/DemoVideo";
import { loadAllScripts, expandThemes } from "./parse-script";
import type { ResolvedScript, Script } from "./schema";
import path from "path";

/**
 * Compute a fallback ResolvedScript for Remotion Studio registration.
 * Recording scenes with "auto" duration get a placeholder of 5 seconds.
 */
function resolveWithDefaults(script: Script): ResolvedScript {
  const fps = script.video.fps;
  const scenes = script.scenes.map((scene) => {
    let durationInFrames: number;
    switch (scene.type) {
      case "title-card":
        durationInFrames = Math.round(scene.duration * fps);
        break;
      case "transition":
        durationInFrames = Math.round(scene.duration * fps);
        break;
      case "recording":
        if (scene.duration === "auto") {
          durationInFrames = 5 * fps; // placeholder
        } else {
          durationInFrames = Math.round(scene.duration * fps);
        }
        break;
    }
    return { scene, durationInFrames };
  });

  return {
    video: script.video,
    branding: script.branding,
    scenes,
    totalDurationInFrames: scenes.reduce((s, r) => s + r.durationInFrames, 0),
  };
}

/**
 * Load scripts and register compositions.
 */
function getCompositions(): Array<{
  id: string;
  script: Script;
  resolved: ResolvedScript;
}> {
  const scriptsDir = path.resolve(import.meta.dirname ?? __dirname, "..", "scripts");

  let allScripts: Script[];
  try {
    allScripts = loadAllScripts(scriptsDir);
  } catch {
    // If scripts dir doesn't exist, return empty for Studio
    return [];
  }

  const expanded = allScripts.flatMap(expandThemes);

  return expanded.map((script) => ({
    id: script.video.id,
    script,
    resolved: resolveWithDefaults(script),
  }));
}

const RemotionRoot: React.FC = () => {
  const compositions = getCompositions();

  return (
    <>
      {compositions.map(({ id, resolved }) => (
        <Composition
          key={id}
          id={id}
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          component={DemoVideo as React.FC<any>}
          durationInFrames={resolved.totalDurationInFrames}
          fps={resolved.video.fps}
          width={resolved.video.width}
          height={resolved.video.height}
          defaultProps={{ script: resolved }}
        />
      ))}
    </>
  );
};

registerRoot(RemotionRoot);
