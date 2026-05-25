import type { NarrationScene } from "../types.ts";

export interface SceneTiming {
  id: string;
  kind: NarrationScene["kind"];
  /** Absolute start frame in the composition. */
  from: number;
  /** Length in frames (audio + hold, with a per-kind minimum). */
  durationFrames: number;
  /** For terminal scenes: cumulative terminal frames elapsed before this scene. */
  termFrom: number;
}

const MIN_FRAMES: Record<NarrationScene["kind"], number> = {
  title: 90,
  prompt: 90,
  outro: 120,
  terminal: 90,
  artifact: 90,
};

export interface Timing {
  scenes: SceneTiming[];
  totalFrames: number;
  totalTermFrames: number;
}

/** Deterministically lay narration scenes onto a frame timeline. */
export function computeTiming(scenes: NarrationScene[], fps: number): Timing {
  const out: SceneTiming[] = [];
  let cursor = 0;
  let termCursor = 0;
  // First pass: total terminal frames (needed by the terminal reveal math).
  for (const s of scenes) {
    if (s.kind !== "terminal") continue;
    const d = Math.max(MIN_FRAMES.terminal, Math.round((s.durationSec + s.holdSec) * fps));
    termCursor += d;
  }
  const totalTermFrames = termCursor;

  termCursor = 0;
  for (const s of scenes) {
    const d = Math.max(MIN_FRAMES[s.kind], Math.round((s.durationSec + s.holdSec) * fps));
    out.push({ id: s.id, kind: s.kind, from: cursor, durationFrames: d, termFrom: s.kind === "terminal" ? termCursor : 0 });
    cursor += d;
    if (s.kind === "terminal") termCursor += d;
  }
  return { scenes: out, totalFrames: cursor, totalTermFrames };
}
