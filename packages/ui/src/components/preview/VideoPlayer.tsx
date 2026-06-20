import React, { useMemo, useRef, useState } from "react";
import { cn } from "../../lib/utils";
import { extentOf, flattenGeometry, type PlacedBlock } from "./geometry";
import OCROverlay from "./OCROverlay";
import SubtitleTimeline from "./SubtitleTimeline";
import { activeCueIndex, collectCues } from "./timeline";
import { useMediaTime } from "./useMediaTime";
import { runsText } from "./renderDoc";
import type { ContentTree, Run } from "./types";

// VideoPlayer — a <video> element wired to a SubtitleTimeline, with the active
// subtitle burned in over the frame and, optionally, the frame's OCR boxes (the
// geometry blocks whose timing spans the playhead) overlaid via OCROverlay. Time
// is element-driven during playback or controlled via `currentTimeMs` (tests /
// Storybook), so the cue sync and frame overlay are demonstrable without real
// media.

export interface VideoPlayerProps {
  /** Resolvable video URL (http(s), blob:, or data: URI). */
  src: string;
  /** The engine output; cues are the timed blocks. */
  tree: ContentTree;
  /** Target variant key shown in the timeline + burned-in subtitle. */
  side?: string;
  /** Controlled playhead (ms); overrides element-driven time when set. */
  currentTimeMs?: number;
  /** Overlay the frame's OCR boxes (timed geometry blocks) at the playhead. */
  showFrameOCR?: boolean;
  /** Poster image URL for the video. */
  poster?: string;
  /**
   * Show the native <video> controls. Default true. Set false when the playhead
   * is driven externally (a controlled `currentTimeMs`) over a non-playable
   * poster — e.g. a canned showcase — so a dead native play button isn't shown.
   */
  controls?: boolean;
  className?: string;
}

/** Pick the cue text for a side, falling back to source when no target exists. */
function cueText(
  node: { source?: Run[]; targets?: Record<string, Run[]> },
  side: string | undefined,
): string {
  if (side && side !== "source") {
    const t = node.targets?.[side];
    if (t && t.length > 0) return runsText(t);
  }
  return runsText(node.source);
}

export default function VideoPlayer({
  src,
  tree,
  side,
  currentTimeMs,
  showFrameOCR = false,
  poster,
  controls = true,
  className,
}: VideoPlayerProps): React.ReactElement {
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const { timeMs, onTimeUpdate, seek } = useMediaTime(videoRef, currentTimeMs);
  const [natural, setNatural] = useState<{ w: number; h: number }>({ w: 0, h: 0 });

  const cues = useMemo(() => collectCues(tree), [tree]);
  const active = activeCueIndex(cues, timeMs);
  const subtitle = active >= 0 ? cueText(cues[active].node, side) : "";

  // Frame OCR: geometry blocks whose timing span contains the playhead.
  const placed: PlacedBlock[] = useMemo(() => flattenGeometry(tree).placed, [tree]);
  const framePlaced = useMemo(
    () =>
      placed.filter(
        (b) =>
          showFrameOCR &&
          b.node.timing &&
          timeMs >= b.node.timing.startMs &&
          timeMs < b.node.timing.endMs,
      ),
    [placed, showFrameOCR, timeMs],
  );
  const { extentW, extentH } = useMemo(
    () => extentOf(framePlaced, natural.w, natural.h),
    [framePlaced, natural.w, natural.h],
  );

  return (
    <div className={cn("flex flex-col gap-3", className)} data-testid="video-player">
      <div className="relative w-full overflow-hidden rounded-md border bg-black">
        <video
          ref={videoRef}
          src={src}
          poster={poster}
          controls={controls}
          onTimeUpdate={onTimeUpdate}
          onLoadedMetadata={(e) =>
            setNatural({ w: e.currentTarget.videoWidth, h: e.currentTarget.videoHeight })
          }
          className="block h-auto w-full"
          data-testid="video-element"
        />
        {framePlaced.length > 0 && (
          <OCROverlay blocks={framePlaced} extentW={extentW} extentH={extentH} />
        )}
        {subtitle && (
          <div
            className="pointer-events-none absolute inset-x-0 bottom-8 flex justify-center px-4"
            data-testid="burned-subtitle"
          >
            <span
              className="max-w-[90%] rounded bg-black/70 px-2 py-1 text-center text-sm text-white"
              lang={side && side !== "source" ? side : undefined}
            >
              {subtitle}
            </span>
          </div>
        )}
      </div>
      <SubtitleTimeline tree={tree} side={side} currentTimeMs={timeMs} onSeek={seek} />
    </div>
  );
}
