import React from "react";
import { interpolate, staticFile } from "remotion";
import { Audio } from "@remotion/media";
import type { NarrationManifest } from "../../types.ts";
import type { SceneSpec } from "../scene-plan.ts";
import { AUDIO_FADE_FRAMES } from "./layout.ts";

/**
 * The narration under each scene: its span of a one-shot track, or its own
 * clip. The track fades in with the first spoken scene and out with the last.
 * Returns the element a scene mounts at its own start frame.
 */
export function narrationAudioFor(
  id: string,
  narration: NarrationManifest,
  scenes: SceneSpec[],
  fps: number,
): (scene: SceneSpec, idx: number) => React.ReactNode {
  const spoken = scenes.map((s, i) => (s.text && (s.audio || s.audioFrom !== undefined) ? i : -1)).filter((i) => i >= 0);
  const firstSpoken = spoken[0] ?? -1;
  const lastSpoken = spoken[spoken.length - 1] ?? -1;
  return (scene, idx) => {
    const fadeIn = idx === firstSpoken;
    const fadeOut = idx === lastSpoken;
    const volumeOver = (segFrames: number) => (f: number) => {
      let v = 1;
      if (fadeIn) v *= interpolate(f, [0, AUDIO_FADE_FRAMES], [0, 1], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
      if (fadeOut) v *= interpolate(f, [segFrames - AUDIO_FADE_FRAMES, segFrames], [1, 0], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
      return v;
    };
    if (narration.fullAudio && scene.audioFrom !== undefined && scene.audioTo !== undefined) {
      const from = Math.round(scene.audioFrom * fps);
      const to = Math.round(scene.audioTo * fps);
      if (to <= from) return null;
      return <Audio src={staticFile(`${id}/${narration.fullAudio}`)} trimBefore={from} trimAfter={to} volume={volumeOver(to - from)} name={`narration:${scene.id}`} />;
    }
    if (!narration.fullAudio && scene.audio) {
      return <Audio src={staticFile(`${id}/${scene.audio}`)} volume={volumeOver(Math.round(scene.durationSec * fps))} name={`narration:${scene.id}`} />;
    }
    return null;
  };
}
