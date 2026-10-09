import React from "react";
import { Pause, Play, SkipBack, SkipForward, StepForward } from "lucide-react";
import type { Chapter } from "../curriculum/types.ts";

// The transport bar under the stage: back, play or pause, run this chapter,
// forward, and a scrubber with one segment per chapter.

export interface TransportProps {
  chapters: readonly Chapter[];
  current: number;
  executed: number;
  playing: boolean;
  busy: boolean;
  ready: boolean;
  onPlay: () => void;
  onPause: () => void;
  onRunCurrent: () => void;
  onBack: () => void;
  onForward: () => void;
  onSeek: (index: number) => void;
}

export default function Transport({
  chapters,
  current,
  executed,
  playing,
  busy,
  ready,
  onPlay,
  onPause,
  onRunCurrent,
  onBack,
  onForward,
  onSeek,
}: TransportProps): React.ReactElement {
  const n = chapters.length;
  const atEnd = executed >= n && current >= n - 1;
  const currentUnrun = current === executed && !atEnd;

  return (
    <div className="kl-transport" role="group" aria-label="Lab transport">
      <div className="kl-transport__buttons">
        <button
          type="button"
          className="kl-tbtn"
          onClick={onBack}
          disabled={!ready || current === 0}
          aria-label="Previous chapter"
          title="Previous chapter (←)"
        >
          <SkipBack size={18} aria-hidden="true" />
        </button>
        {playing ? (
          <button
            type="button"
            className="kl-tbtn kl-tbtn--main"
            onClick={onPause}
            aria-label="Pause"
            title="Pause (space)"
          >
            <Pause size={22} aria-hidden="true" fill="currentColor" />
          </button>
        ) : (
          <button
            type="button"
            className="kl-tbtn kl-tbtn--main"
            onClick={onPlay}
            disabled={!ready || atEnd}
            aria-label="Play: run the chapters from here"
            title="Play (space)"
          >
            <Play size={22} aria-hidden="true" fill="currentColor" />
          </button>
        )}
        <button
          type="button"
          className="kl-tbtn"
          onClick={onRunCurrent}
          disabled={!ready || busy || !currentUnrun || playing}
          aria-label="Run this chapter"
          title="Run this chapter (enter)"
        >
          <StepForward size={18} aria-hidden="true" />
        </button>
        <button
          type="button"
          className="kl-tbtn"
          onClick={onForward}
          disabled={!ready || busy || current >= n - 1}
          aria-label="Next chapter"
          title="Next chapter (→)"
        >
          <SkipForward size={18} aria-hidden="true" />
        </button>
      </div>

      <div className="kl-scrub" role="list" aria-label="Chapters">
        {chapters.map((ch, i) => {
          const state = i < executed ? "done" : i === executed ? "next" : "pending";
          return (
            <button
              key={ch.id}
              type="button"
              role="listitem"
              className={`kl-scrub__seg kl-scrub__seg--${state}${i === current ? " kl-scrub__seg--current" : ""}`}
              onClick={() => onSeek(i)}
              disabled={!ready || busy}
              aria-label={`Chapter ${i + 1}: ${ch.title}${i < executed ? " (done)" : ""}`}
              aria-current={i === current ? "step" : undefined}
              title={`${i + 1}. ${ch.title}`}
            />
          );
        })}
      </div>

      <div className="kl-transport__counter" aria-live="polite">
        {current + 1} / {n}
      </div>
    </div>
  );
}
