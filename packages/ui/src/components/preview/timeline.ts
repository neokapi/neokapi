// Shared temporal helpers for the time-based views (SubtitleTimeline,
// AudioPlayer, VideoPlayer). A timed source (SRT/VTT/TTML, an audio or video
// transcript) anchors each block to a [startMs, endMs) cue span via its
// TimingView. Turning those blocks into an ordered cue list, finding the active
// cue at a given playhead, and formatting a timecode are the same operations
// across all three components, so they live here once.

import type { ContentNode, ContentTree, TimingView } from "./types";

/** A timed block lifted out of the tree, in start-time order. */
export interface Cue {
  node: ContentNode;
  timing: TimingView;
  /** 0-based position in the start-ordered cue list. */
  index: number;
}

/**
 * collectCues walks the tree in document order, keeps every block carrying a
 * `timing` anchor, sorts them by start time (then end time), and assigns a
 * stable 0-based index. Blocks without timing are ignored.
 */
export function collectCues(tree: ContentTree): Cue[] {
  const timed: { node: ContentNode; timing: TimingView }[] = [];
  const walk = (n: ContentNode) => {
    if (n.kind === "block" && n.timing) timed.push({ node: n, timing: n.timing });
    n.children?.forEach(walk);
  };
  tree.root.forEach(walk);
  timed.sort((a, b) => a.timing.startMs - b.timing.startMs || a.timing.endMs - b.timing.endMs);
  return timed.map((c, index) => ({ ...c, index }));
}

/**
 * activeCueIndex returns the index of the cue whose [startMs, endMs) span
 * contains the playhead (ms), or -1 when none does. The span is half-open so a
 * cue ending exactly when the next begins never both match. When cues overlap,
 * the latest-starting matching cue wins (the one most recently entered).
 */
export function activeCueIndex(cues: Cue[], ms: number): number {
  let found = -1;
  for (const c of cues) {
    if (ms >= c.timing.startMs && ms < c.timing.endMs) found = c.index;
  }
  return found;
}

/**
 * formatTimecode renders a millisecond offset as HH:MM:SS.mmm (zero-padded). A
 * negative input clamps to 0. This is the SRT/VTT-style timecode the cue list
 * shows next to each line.
 */
export function formatTimecode(ms: number): string {
  const total = Math.max(0, Math.round(ms));
  const millis = total % 1000;
  const totalSec = Math.floor(total / 1000);
  const seconds = totalSec % 60;
  const minutes = Math.floor(totalSec / 60) % 60;
  const hours = Math.floor(totalSec / 3600);
  const p2 = (n: number) => String(n).padStart(2, "0");
  const p3 = (n: number) => String(n).padStart(3, "0");
  return `${p2(hours)}:${p2(minutes)}:${p2(seconds)}.${p3(millis)}`;
}

/** A compact duration label for a cue, e.g. "1.4s". */
export function formatDuration(ms: number): string {
  return `${(Math.max(0, ms) / 1000).toFixed(1)}s`;
}
