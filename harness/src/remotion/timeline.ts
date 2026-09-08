import type { NarrationScene, TimelineEvent } from "../types.ts";

/** A narration scene with the two authored fields the timeline reads. */
export type TimedScene = NarrationScene & { hold?: number; through?: number };

export interface SceneTiming {
  id: string;
  kind: NarrationScene["kind"];
  /** Absolute start frame in the composition. */
  from: number;
  /** Length in frames, including the frames the next transition overlaps. */
  durationFrames: number;
  /** Frames of the transition that leads into this scene (0 for a cut). */
  transitionBefore: number;
  /** Frames of the transition that leads out of this scene (0 for a cut). */
  transitionAfter: number;
  /** For terminal scenes: cumulative terminal frames elapsed before this scene. */
  termFrom: number;
  /**
   * For terminal scenes: how many events are revealed at the scene's start and
   * at its end, in event units (fractional: the fraction is the event being
   * typed). Anchored by the scenes' `through` declarations.
   */
  revealStart: number;
  revealEnd: number;
}

const MIN_FRAMES: Record<NarrationScene["kind"], number> = {
  title: 90,
  prompt: 90,
  outro: 120,
  terminal: 90,
  artifact: 90,
  desktop: 90,
};

export interface Timing {
  scenes: SceneTiming[];
  totalFrames: number;
  totalTermFrames: number;
}

/** Frames two adjacent scenes overlap; 0 is a cut. */
export type TransitionRule = (prev: TimedScene, next: TimedScene) => number;

export interface TimingOptions {
  /** Which transition, if any, sits between two scenes. Default: cuts only. */
  transition?: TransitionRule;
  /** The terminal events, so `through` anchors can be turned into event counts. */
  events?: TimelineEvent[];
}

/**
 * The number of events revealed once script step `step` (1-based, counting
 * commands and comments) has run: the step's own line plus every output line
 * that follows it. A step past the end reveals everything.
 */
export function eventCountForStep(events: TimelineEvent[], step: number): number {
  let seen = 0;
  for (let i = 0; i < events.length; i++) {
    const ev = events[i]!;
    if (ev.kind !== "command" && ev.kind !== "comment") continue;
    seen++;
    if (seen < step) continue;
    let end = i + 1;
    while (end < events.length && events[end]!.kind === "output") end++;
    return end;
  }
  return events.length;
}

/** The frames of a scene before any transition is added: audio, hold, floor. */
function baseFrames(s: TimedScene, fps: number): number {
  const audio = Math.round((s.durationSec + s.holdSec) * fps);
  const hold = s.hold ? Math.round(s.hold * fps) : 0;
  return Math.max(MIN_FRAMES[s.kind], audio, hold);
}

/**
 * Deterministically lay narration scenes onto a frame timeline.
 *
 * A scene lasts its narration (plus hold, with a per-kind floor) and then the
 * frames of the transition into the next scene, so the next scene's narration
 * starts exactly when the transition completes: scene i starts at the sum of
 * the bare lengths before it, and the composition is that sum. A transition
 * therefore shortens the whole by its length, as TransitionSeries would.
 */
export function computeTiming(scenes: TimedScene[], fps: number, opts: TimingOptions = {}): Timing {
  const transition = opts.transition ?? (() => 0);
  const bases = scenes.map((s) => baseFrames(s, fps));
  const after = scenes.map((s, i) => (i + 1 < scenes.length ? Math.max(0, Math.round(transition(s, scenes[i + 1]!))) : 0));

  const out: SceneTiming[] = [];
  let cursor = 0;
  let termCursor = 0;
  for (let i = 0; i < scenes.length; i++) {
    const s = scenes[i]!;
    const d = bases[i]! + after[i]!;
    const isTerm = s.kind === "terminal";
    out.push({
      id: s.id,
      kind: s.kind,
      from: cursor,
      durationFrames: d,
      transitionBefore: i > 0 ? after[i - 1]! : 0,
      transitionAfter: after[i]!,
      termFrom: isTerm ? termCursor : 0,
      revealStart: 0,
      revealEnd: 0,
    });
    cursor += bases[i]!;
    if (isTerm) termCursor += d;
  }
  const totalFrames = cursor;
  const totalTermFrames = termCursor;

  // Reveal anchors: (terminal frame, events revealed) keypoints, linear between.
  const events = opts.events ?? [];
  const N = events.length;
  const keys: Array<[number, number]> = [[0, 0]];
  let anchoredLast = false;
  for (let i = 0; i < scenes.length; i++) {
    const s = scenes[i]!;
    if (s.kind !== "terminal") continue;
    const t = out[i]!;
    anchoredLast = false;
    if (s.through !== undefined) {
      keys.push([t.termFrom + t.durationFrames, eventCountForStep(events, s.through)]);
      anchoredLast = true;
    }
  }
  if (!anchoredLast) keys.push([totalTermFrames, N + 0.5]);
  const valueAt = (frame: number): number => {
    if (totalTermFrames <= 0) return N;
    let prev = keys[0]!;
    for (let k = 1; k < keys.length; k++) {
      const next = keys[k]!;
      if (frame <= next[0]) {
        const span = next[0] - prev[0];
        const p = span > 0 ? (frame - prev[0]) / span : 1;
        return Math.min(N, prev[1] + (next[1] - prev[1]) * Math.max(0, Math.min(1, p)));
      }
      prev = next;
    }
    return Math.min(N, prev[1]);
  };
  for (const t of out) {
    if (t.kind !== "terminal") continue;
    t.revealStart = valueAt(t.termFrom);
    t.revealEnd = valueAt(t.termFrom + t.durationFrames);
  }
  return { scenes: out, totalFrames, totalTermFrames };
}
