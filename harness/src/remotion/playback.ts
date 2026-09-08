/**
 * How a recorded beat fills its scene, as arithmetic the composition and the
 * tests share: the playback plan, and where the recorded clicks land.
 */
import type { ScreencastBeat } from "../types.ts";

/** Slowest the picture may play before the last frame is held instead. */
export const MIN_PLAYBACK_RATE = 0.8;
/** Fastest the picture may play to fit a scene shorter than its beat. */
export const MAX_PLAYBACK_RATE = 2;

export interface PlayPlan {
  rate: number;
  /** Frames the picture moves; the rest of the scene holds the last of them. */
  playFrames: number;
  heldFrames: number;
}

/**
 * Near real time the slice plays at its natural rate (never slower than
 * MIN_PLAYBACK_RATE, never faster than MAX); when the narration outruns the
 * picture, the picture plays at 1x and its last frame holds for the
 * remainder, so a cursor never crawls.
 */
export function planPlayback(sliceSec: number, sceneFrames: number, fps: number): PlayPlan {
  const slice = Math.max(0.1, sliceSec);
  const sceneSec = Math.max(1 / fps, sceneFrames / fps);
  const ratio = slice / sceneSec;
  if (ratio >= MIN_PLAYBACK_RATE) {
    return { rate: Math.min(MAX_PLAYBACK_RATE, ratio), playFrames: sceneFrames, heldFrames: 0 };
  }
  const playFrames = Math.max(1, Math.min(sceneFrames, Math.round(slice * fps)));
  return { rate: 1, playFrames, heldFrames: sceneFrames - playFrames };
}

/** Scene frames at which the recorded clicks inside the played slice land. */
export function clickFrames(clicks: number[] | undefined, beat: Pick<ScreencastBeat, "tStart" | "tEnd">, plan: PlayPlan, fps: number): number[] {
  if (!clicks || clicks.length === 0) return [];
  return clicks
    .filter((c) => c >= beat.tStart && c < beat.tEnd)
    .map((c) => Math.round(((c - beat.tStart) / plan.rate) * fps))
    .filter((f) => f >= 0 && f < plan.playFrames);
}
