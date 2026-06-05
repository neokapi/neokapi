import React from "react";
import { ChevronLeft, ChevronRight, Pause, Play, SkipBack } from "lucide-react";
import type { FlowPlaybackState } from "./useFlowPlayback";
import styles from "./styles.module.css";

interface StepControlsProps {
  state: FlowPlaybackState;
  onStepPrev: () => void;
  onStepNext: () => void;
  onPlay: () => void;
  onPause: () => void;
  onReset: () => void;
  onSeek: (frameIndex: number) => void;
  onSetSpeed: (fps: number) => void;
}

const SPEEDS = [1, 2, 4, 8];

const LEGEND_ITEMS = [
  { label: "Block", cls: "bg-blue-500" },
  { label: "Layer", cls: "bg-emerald-500" },
  { label: "Group", cls: "bg-violet-500" },
  { label: "Data", cls: "bg-slate-400" },
  { label: "Media", cls: "bg-amber-500" },
];

export default function StepControls({
  state,
  onStepPrev,
  onStepNext,
  onPlay,
  onPause,
  onReset,
  onSeek,
  onSetSpeed,
}: StepControlsProps): React.ReactElement {
  return (
    <div className={styles.stepBar}>
      <button
        className={styles.stepButton}
        onClick={onReset}
        disabled={state.atStart}
        title="Back to start"
      >
        <SkipBack size={15} />
      </button>
      <button
        className={styles.stepButton}
        onClick={onStepPrev}
        disabled={state.atStart}
        title="Previous step"
      >
        <ChevronLeft size={15} /> Prev
      </button>
      <button
        className={styles.stepButtonPrimary}
        onClick={onStepNext}
        disabled={state.atEnd}
        title="Advance one step"
      >
        Next <ChevronRight size={15} />
      </button>

      <span className={styles.divider} />

      <button
        className={styles.stepButton}
        onClick={state.playing ? onPause : onPlay}
        disabled={state.atEnd && !state.playing}
        title={state.playing ? "Pause" : "Play through"}
      >
        {state.playing ? <Pause size={15} /> : <Play size={15} />}
        {state.playing ? "Pause" : "Play"}
      </button>
      <div className={styles.speedGroup}>
        {SPEEDS.map((fps) => (
          <button
            key={fps}
            className={`${styles.speedButton} ${state.speed === fps ? styles.speedButtonActive : ""}`}
            onClick={() => onSetSpeed(fps)}
            title={`${fps} steps per second`}
          >
            {fps}×
          </button>
        ))}
      </div>

      <span className={styles.frameCounter}>
        step {state.frameIndex} / {state.frameCount - 1}
      </span>

      <input
        type="range"
        className={styles.scrubber}
        min={0}
        max={Math.max(0, state.frameCount - 1)}
        value={state.frameIndex}
        onChange={(e) => onSeek(Number(e.target.value))}
        aria-label="Step position"
      />

      <div className={styles.legend}>
        {LEGEND_ITEMS.map((item) => (
          <span key={item.label} className={styles.legendItem}>
            <span className={`inline-block size-2.5 rounded-full ${item.cls}`} />
            {item.label}
          </span>
        ))}
      </div>
    </div>
  );
}
