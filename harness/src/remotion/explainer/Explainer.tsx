/**
 * An explainer: a narrated video of drawn diagram beats, with no capture and
 * no terminal. demo.yaml sets `terminal: explainer` and lists `kind: diagram`
 * scenes, each naming one of the explainer's beats (EXPLAINERS below). The
 * timing is the template's: each beat lasts its measured narration, held to
 * the scene's `hold`, with the template's fades between beats.
 *
 * The spoken words are not burned in as captions: the beats already draw
 * their words, and the lower-third pill would sit over the diagrams.
 * captions.json still carries them for a player's text track.
 */
import React from "react";
import { AbsoluteFill, staticFile, useVideoConfig, type CalculateMetadataFunction } from "remotion";
import type { BeatsFile, CaptionsFile, NarrationManifest } from "../../types.ts";
import { computeTiming } from "../timeline.ts";
import { mergeScenes, presentationBetween, transitionFrames } from "../scene-plan.ts";
import { SceneSeries, type SceneSlot } from "../SceneSeries.tsx";
import { narrationAudioFor } from "../components/NarrationAudio.tsx";
import { FPS, HEIGHT, WIDTH, setTheme, theme, type ThemeMode } from "../components/theme.ts";
import { cueFrames, type BeatProps } from "./primitives.tsx";
import { E2_BEATS } from "./e2.tsx";

/** Every explainer, by demo id: the diagrams its demo.yaml may name. */
export const EXPLAINERS: Record<string, Record<string, React.FC<BeatProps>>> = {
  "e2-communication-is-context": E2_BEATS,
};

export type ExplainerProps = {
  id: string;
  narration?: NarrationManifest | null;
  beats?: BeatsFile | null;
  captions?: CaptionsFile | null;
  themeMode?: ThemeMode;
  locale?: string;
  stamp?: string;
};

async function fetchJson<T>(rel: string): Promise<T | null> {
  try {
    const res = await fetch(staticFile(rel));
    if (!res.ok) return null;
    return (await res.json()) as T;
  } catch {
    return null;
  }
}

export const explainerCalcMeta: CalculateMetadataFunction<ExplainerProps> = async ({ props }) => {
  const { id } = props;
  const narration = await fetchJson<NarrationManifest>(`${id}/narration.json`);
  const beats = await fetchJson<BeatsFile>(`${id}/beats.json`);
  const captions = narration?.captions ? await fetchJson<CaptionsFile>(`${id}/${narration.captions}`) : null;
  // Before narration exists, the beats preview silent at their holds and floors.
  const silent: NarrationManifest = { id, backend: "none", voice: "none", scenes: [] };
  const timing = computeTiming(mergeScenes(narration ?? silent, beats), FPS, { transition: transitionFrames });
  return {
    durationInFrames: Math.max(FPS, timing.totalFrames),
    fps: FPS,
    width: WIDTH,
    height: HEIGHT,
    props: { ...props, narration, beats, captions },
  };
};

export const Explainer: React.FC<ExplainerProps> = ({ id, narration, beats, captions, themeMode, stamp }) => {
  setTheme(themeMode ?? "dark");
  const { fps } = useVideoConfig();
  const diagrams = EXPLAINERS[id] ?? {};
  const track: NarrationManifest = narration ?? { id, backend: "none", voice: "none", scenes: [] };
  const scenes = mergeScenes(track, beats ?? null);
  const timing = computeTiming(scenes, fps, { transition: transitionFrames });
  const sceneAudio = narrationAudioFor(id, track, scenes, fps);
  const captionsOf = new Map((captions?.scenes ?? []).map((s) => [s.id, s.captions] as const));

  const slots: SceneSlot[] = scenes.map((scene, idx) => {
    const t = timing.scenes[idx]!;
    const Beat = scene.diagram ? diagrams[scene.diagram] : undefined;
    return {
      key: scene.id,
      name: `diagram:${scene.id}`,
      from: t.from,
      durationInFrames: t.durationFrames,
      transitionBefore: t.transitionBefore,
      transitionAfter: t.transitionAfter,
      presentationBefore: idx > 0 ? presentationBetween(scenes[idx - 1]!, scene) : undefined,
      presentationAfter: idx + 1 < scenes.length ? presentationBetween(scene, scenes[idx + 1]!) : undefined,
      children: (
        <AbsoluteFill style={{ background: theme.panel }}>
          {sceneAudio(scene, idx)}
          {Beat ? <Beat cue={cueFrames(captionsOf.get(scene.id) ?? [], fps)} durationFrames={t.durationFrames} /> : null}
        </AbsoluteFill>
      ),
    };
  });

  return (
    <AbsoluteFill style={{ background: theme.panel, fontFamily: theme.fontSans }}>
      <SceneSeries slots={slots} />
      {stamp ? (
        <div
          style={{
            position: "absolute",
            right: 18,
            bottom: 14,
            zIndex: 100,
            fontFamily: theme.fontMono,
            fontSize: 13,
            lineHeight: 1,
            color: theme.text,
            opacity: 0.32,
          }}
        >
          {stamp}
        </div>
      ) : null}
    </AbsoluteFill>
  );
};
