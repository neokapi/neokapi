import React from "react";
import { Composition, staticFile, type CalculateMetadataFunction } from "remotion";
import type { CapturedArtifact, DemoCapture, NarrationManifest } from "../types.ts";
import { Demo, type DemoProps } from "./Demo.tsx";
import { computeTiming } from "./timeline.ts";
import { FPS, WIDTH, HEIGHT } from "./components/theme.ts";
import { DEMOS } from "./registry.generated.ts";

async function fetchJson<T>(rel: string): Promise<T | null> {
  try {
    const res = await fetch(staticFile(rel));
    if (!res.ok) return null;
    return (await res.json()) as T;
  } catch {
    return null;
  }
}

/** Fallback narration so a demo still previews in Studio before TTS has run. */
function fallbackNarration(capture: DemoCapture): NarrationManifest {
  const termSec = Math.max(20, Math.min(90, capture.events.length * 2.2));
  return {
    id: capture.id,
    backend: "none",
    voice: "—",
    scenes: [
      { id: "title", kind: "title", text: "", caption: capture.title, durationSec: 4, holdSec: 0 },
      { id: "session", kind: "terminal", text: "", caption: capture.subtitle, durationSec: termSec, holdSec: 0 },
      { id: "outro", kind: "outro", text: "", caption: "", durationSec: 4, holdSec: 0 },
    ],
  };
}

const calcMeta: CalculateMetadataFunction<DemoProps> = async ({ props }) => {
  const id = props.id;
  const capture = await fetchJson<DemoCapture>(`${id}/capture.json`);
  if (!capture) {
    // Nothing captured yet — render a 1s placeholder so Studio doesn't crash.
    return { durationInFrames: FPS, fps: FPS, width: WIDTH, height: HEIGHT, props };
  }
  const narration = (await fetchJson<NarrationManifest>(`${id}/narration.json`)) ?? fallbackNarration(capture);
  const artifacts = (await fetchJson<CapturedArtifact[]>(`${id}/artifacts.json`)) ?? [];
  const timing = computeTiming(narration.scenes, FPS);
  return {
    durationInFrames: Math.max(FPS, timing.totalFrames),
    fps: FPS,
    width: WIDTH,
    height: HEIGHT,
    props: { ...props, capture, narration, artifacts },
  };
};

export const RemotionRoot: React.FC = () => {
  return (
    <>
      {DEMOS.map((d) => (
        <Composition
          key={d.id}
          id={d.id}
          component={Demo}
          durationInFrames={FPS}
          fps={FPS}
          width={WIDTH}
          height={HEIGHT}
          defaultProps={{ id: d.id, capture: undefined as any, narration: undefined as any, artifacts: [] as CapturedArtifact[], themeMode: "dark" as const }}
          calculateMetadata={calcMeta}
        />
      ))}
    </>
  );
};
