import React, { useRef } from "react";
import { cn } from "../../lib/utils";
import SubtitleTimeline from "./SubtitleTimeline";
import { useMediaTime } from "./useMediaTime";
import type { ContentTree } from "./types";

// AudioPlayer — an <audio> element wired to a SubtitleTimeline. As playback
// progresses the cue spanning the playhead is highlighted; clicking a cue seeks
// the audio to that cue's start. Time can be driven by the element (real
// playback) or by the controlled `currentTimeMs` prop (tests / Storybook),
// keeping the cue-sync demonstrable without real media.

export interface AudioPlayerProps {
  /** Resolvable audio URL (http(s), blob:, or data: URI). */
  src: string;
  /** The engine output; cues are the timed blocks. */
  tree: ContentTree;
  /** Target variant key to show beside the source in the timeline. */
  side?: string;
  /** Controlled playhead (ms); overrides element-driven time when set. */
  currentTimeMs?: number;
  className?: string;
}

export default function AudioPlayer({
  src,
  tree,
  side,
  currentTimeMs,
  className,
}: AudioPlayerProps): React.ReactElement {
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const { timeMs, onTimeUpdate, seek } = useMediaTime(audioRef, currentTimeMs);

  return (
    <div className={cn("flex flex-col gap-3", className)} data-testid="audio-player">
      <audio
        ref={audioRef}
        src={src}
        controls
        onTimeUpdate={onTimeUpdate}
        className="w-full"
        data-testid="audio-element"
      />
      <SubtitleTimeline tree={tree} side={side} currentTimeMs={timeMs} onSeek={seek} />
    </div>
  );
}
