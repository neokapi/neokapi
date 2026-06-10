// TracePanel — the playback transport for a run trace, docked under the
// canvas. The designed flow IS the run flow: stepping the cursor replays the
// trace's events on the same nodes the user composed (active-node highlight,
// part counts), so there is no separate run view.

import { useCallback, useEffect, useRef, useState } from "react";
import { Play, Pause, SkipBack, SkipForward, RotateCcw, X } from "lucide-react";
import { Button, cn } from "@neokapi/ui-primitives";
import type { TraceEvent } from "./traceTypes";

export interface TracePanelProps {
  /** Events remapped onto editor node ids, in time order. */
  events: TraceEvent[];
  /** Cursor: how many events have been applied (0..events.length). */
  cursor: number;
  onCursorChange: (cursor: number) => void;
  /** Total run duration in microseconds. */
  durationUs?: number;
  /** Dismiss the trace (leave run-review mode). */
  onClose: () => void;
}

/** Wall-clock label for the event at the cursor. */
function tsLabel(events: TraceEvent[], cursor: number, durationUs?: number): string {
  const us = cursor > 0 ? events[Math.min(cursor, events.length) - 1].ts : 0;
  const total = durationUs ?? (events.length > 0 ? events[events.length - 1].ts : 0);
  const fmt = (v: number) => (v >= 1000 ? `${(v / 1000).toFixed(1)} ms` : `${v} µs`);
  return `${fmt(us)} / ${fmt(total)}`;
}

export function TracePanel({
  events,
  cursor,
  onCursorChange,
  durationUs,
  onClose,
}: TracePanelProps) {
  const [playing, setPlaying] = useState(false);
  const playRef = useRef<number | null>(null);
  const total = events.length;
  const done = cursor >= total;

  const stop = useCallback(() => {
    setPlaying(false);
  }, []);

  // Advance the cursor at a readable pace while playing (events are bursty in
  // real time, so playback is event-paced, not wall-clock-paced).
  useEffect(() => {
    if (!playing) return;
    if (cursor >= total) {
      setPlaying(false);
      return;
    }
    playRef.current = window.setTimeout(() => onCursorChange(cursor + 1), 160);
    return () => {
      if (playRef.current !== null) window.clearTimeout(playRef.current);
    };
  }, [playing, cursor, total, onCursorChange]);

  return (
    <div className="flex items-center gap-2 border-t border-border bg-background px-3 py-1.5">
      <span className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
        Run
      </span>
      <Button
        variant="ghost"
        size="xs"
        className="h-6 px-1.5"
        onClick={() => {
          stop();
          onCursorChange(0);
        }}
        title="Restart"
        aria-label="Restart playback"
      >
        <RotateCcw size={12} />
      </Button>
      <Button
        variant="ghost"
        size="xs"
        className="h-6 px-1.5"
        onClick={() => {
          stop();
          onCursorChange(Math.max(0, cursor - 1));
        }}
        title="Step back"
        aria-label="Step back"
      >
        <SkipBack size={12} />
      </Button>
      <Button
        variant={playing ? "outline" : "default"}
        size="xs"
        className="h-6 px-2"
        onClick={() => {
          if (done) onCursorChange(0);
          setPlaying((p) => !p);
        }}
        title={playing ? "Pause" : "Play"}
        aria-label={playing ? "Pause playback" : "Play"}
      >
        {playing ? <Pause size={12} /> : <Play size={12} />}
      </Button>
      <Button
        variant="ghost"
        size="xs"
        className="h-6 px-1.5"
        onClick={() => {
          stop();
          onCursorChange(Math.min(total, cursor + 1));
        }}
        title="Step forward"
        aria-label="Step forward"
      >
        <SkipForward size={12} />
      </Button>

      <input
        type="range"
        min={0}
        max={total}
        value={cursor}
        onChange={(e) => {
          stop();
          onCursorChange(Number(e.target.value));
        }}
        className="h-1 flex-1 accent-[var(--ring)]"
        aria-label="Playback position"
      />

      <span
        className={cn(
          "min-w-[110px] text-right font-mono text-[10px]",
          done ? "text-foreground" : "text-muted-foreground",
        )}
      >
        {cursor}/{total} · {tsLabel(events, cursor, durationUs)}
      </span>

      <Button
        variant="ghost"
        size="xs"
        className="h-6 px-1.5"
        onClick={onClose}
        title="Dismiss the run"
        aria-label="Dismiss the run"
      >
        <X size={12} />
      </Button>
    </div>
  );
}
