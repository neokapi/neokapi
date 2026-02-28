import React from "react";
import { Composition, getInputProps, registerRoot } from "remotion";
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
 * Load scripts from filesystem. Works in Remotion Studio (Node.js);
 * returns [] in the webpack bundle where fs/path are unavailable.
 */
function getCompositions(): Array<{
  id: string;
  resolved: ResolvedScript;
}> {
  try {
    const scriptsDir = path.resolve(import.meta.dirname ?? __dirname, "..", "scripts");
    const allScripts = loadAllScripts(scriptsDir);
    const expanded = allScripts.flatMap(expandThemes);

    return expanded.map((script) => ({
      id: script.video.id,
      resolved: resolveWithDefaults(script),
    }));
  } catch {
    return [];
  }
}

const RemotionRoot: React.FC = () => {
  // Try filesystem-based loading (works in Remotion Studio dev mode)
  const compositions = getCompositions();

  if (compositions.length > 0) {
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
  }

  // Webpack bundle: fs/path unavailable. Use inputProps from render.ts
  // which passes { script: ResolvedScript } via selectComposition().
  const props = getInputProps() as { script?: ResolvedScript };
  const script = props?.script;

  if (!script) {
    return null;
  }

  return (
    <Composition
      id={script.video.id}
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      component={DemoVideo as React.FC<any>}
      durationInFrames={script.totalDurationInFrames}
      fps={script.video.fps}
      width={script.video.width}
      height={script.video.height}
      defaultProps={{ script }}
    />
  );
};

registerRoot(RemotionRoot);
