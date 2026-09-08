import React from "react";
import { Composition, Folder, staticFile, type CalculateMetadataFunction } from "remotion";
import type { BeatsFile, CapturedArtifact, CaptionsFile, DemoCapture, NarrationManifest, Screencast } from "../types.ts";
import { Demo, type DemoProps } from "./Demo.tsx";
import { Sizzle, sizzleCalcMeta } from "./compositions/Sizzle.tsx";
import { MultimodalSample, SAMPLE_FPS, SAMPLE_WIDTH, SAMPLE_HEIGHT, SAMPLE_FRAMES } from "./compositions/MultimodalSample.tsx";
import { computeTiming } from "./timeline.ts";
import { mergeScenes, transitionFrames } from "./scene-plan.ts";
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

/**
 * A stand-in capture for a demo this machine has not recorded. The registry
 * lists every authored demo, not the locally captured ones, so a composition
 * can be opened before its capture exists; it says so on screen rather than
 * failing on an absent capture. (The render stage skips such a demo outright.)
 */
function placeholderCapture(id: string, title: string): DemoCapture {
  return {
    id,
    title,
    subtitle: "not captured on this machine",
    aspects: [],
    prompt: "",
    terminal: "shell",
    brand: "kapi",
    events: [{ i: 0, kind: "comment", text: `no capture yet. run: pnpm run demo ${id} --only=capture` }],
    meta: { model: "shell", durationMs: 0, numTurns: 0, capturedAt: "", errors: [] },
  };
}

/** Fallback narration so a demo still previews in Studio before TTS has run. */
function fallbackNarration(capture: DemoCapture): NarrationManifest {
  const termSec = Math.max(20, Math.min(90, capture.events.length * 2.2));
  return {
    id: capture.id,
    backend: "none",
    voice: "none",
    scenes: [
      { id: "title", kind: "title", text: "", caption: "", durationSec: 4, holdSec: 0 },
      { id: "session", kind: "terminal", text: "", caption: "", durationSec: termSec, holdSec: 0 },
      { id: "outro", kind: "outro", text: "", caption: "", durationSec: 4, holdSec: 0 },
    ],
  };
}

const calcMeta: CalculateMetadataFunction<DemoProps> = async ({ props }) => {
  const id = props.id;
  const title = DEMOS.find((d) => d.id === id)?.title ?? id;
  const capture = (await fetchJson<DemoCapture>(`${id}/capture.json`)) ?? placeholderCapture(id, title);
  // Locale-suffixed narration (narration-nb.json) when the render passes a
  // non-default locale; the English narration.json otherwise. No silent
  // cross-locale fallback here: render.ts refuses to render a locale whose
  // narration is missing, so a missing file only means a Studio preview.
  const locale = typeof props.locale === "string" && props.locale !== "en" ? props.locale : "";
  const suffix = locale ? `-${locale}` : "";
  const narration =
    (locale ? await fetchJson<NarrationManifest>(`${id}/narration${suffix}.json`) : null) ??
    (await fetchJson<NarrationManifest>(`${id}/narration.json`)) ??
    fallbackNarration(capture);
  const beats = (locale ? await fetchJson<BeatsFile>(`${id}/beats${suffix}.json`) : null) ?? (await fetchJson<BeatsFile>(`${id}/beats.json`));
  const captions = narration.captions ? await fetchJson<CaptionsFile>(`${id}/${narration.captions}`) : null;
  const artifacts = (await fetchJson<CapturedArtifact[]>(`${id}/artifacts.json`)) ?? [];
  const screencast = await fetchJson<Screencast>(`${id}/screencast.json`);
  const timing = computeTiming(mergeScenes(narration, beats), FPS, { transition: transitionFrames, events: capture.events });
  return {
    durationInFrames: Math.max(FPS, timing.totalFrames),
    fps: FPS,
    width: WIDTH,
    height: HEIGHT,
    props: { ...props, capture, narration, beats, captions, artifacts, screencast },
  };
};

export const RemotionRoot: React.FC = () => {
  return (
    <>
      <Folder name="Demos">
        {DEMOS.filter((d) => d.id !== "bowrain-sizzle").map((d) => (
          <Composition
            key={d.id}
            id={d.id}
            component={Demo}
            durationInFrames={FPS}
            fps={FPS}
            width={WIDTH}
            height={HEIGHT}
            defaultProps={{ id: d.id }}
            calculateMetadata={calcMeta}
          />
        ))}
      </Folder>
      <Folder name="Reels">
        {/* Landing sizzle: a montage of the bowrain feature screencasts (not a
            captured demo). Its own component + metadata loader; duration is
            computed from the clip plan in Sizzle.tsx. */}
        <Composition
          id="bowrain-sizzle"
          component={Sizzle}
          durationInFrames={FPS}
          fps={FPS}
          width={WIDTH}
          height={HEIGHT}
          defaultProps={{ id: "bowrain-sizzle" }}
          calculateMetadata={sizzleCalcMeta}
        />
      </Folder>
      <Folder name="Samples">
        {/* A short, self-contained sample clip for the in-browser audio/video labs
            (rendered to web/static/samples/multimodal-sample.mp4). Title cards for
            OCR over a Gemini-TTS narration for speech recognition. */}
        <Composition
          id="multimodal-sample"
          component={MultimodalSample}
          durationInFrames={SAMPLE_FRAMES}
          fps={SAMPLE_FPS}
          width={SAMPLE_WIDTH}
          height={SAMPLE_HEIGHT}
        />
      </Folder>
    </>
  );
};
