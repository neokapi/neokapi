import React from 'react';
import type { PlaybackState, TraceEvent } from './_types';
import styles from './_index.module.css';

interface TimelineControlsProps {
  state: PlaybackState;
  events: TraceEvent[];
  onPlay: () => void;
  onPause: () => void;
  onStep: () => void;
  onSeek: (time: number) => void;
  onSetSpeed: (speed: number) => void;
  onReset: () => void;
}

const SPEEDS: { value: number; label: string }[] = [
  { value: 0.01, label: '1/100x' },
  { value: 0.1, label: '1/10x' },
  { value: 0.2, label: '1/5x' },
  { value: 1, label: '1x' },
];

const LEGEND_ITEMS = [
  { label: 'Block', color: '#3b82f6' },
  { label: 'Layer', color: '#22c55e' },
  { label: 'Data', color: '#94a3b8' },
  { label: 'Media', color: '#f59e0b' },
];

function formatTime(us: number): string {
  if (us < 1000) return `${Math.round(us)}\u00b5s`;
  return `${(us / 1000).toFixed(1)}ms`;
}

export default function TimelineControls({
  state,
  events,
  onPlay,
  onPause,
  onStep,
  onSeek,
  onSetSpeed,
  onReset,
}: TimelineControlsProps): React.ReactElement {
  return (
    <div className={styles.controls}>
      <button
        className={styles.controlButton}
        onClick={state.playing ? onPause : onPlay}
        title={state.playing ? 'Pause' : 'Play'}
      >
        {state.playing ? '\u23f8' : '\u25b6'}
      </button>
      <button
        className={styles.controlButton}
        onClick={onReset}
        title="Reset"
      >
        {'\u23ee'}
      </button>
      <button
        className={styles.controlButton}
        onClick={onStep}
        title="Step to next event"
      >
        {'\u23ed'}
      </button>

      <div className={styles.speedGroup}>
        {SPEEDS.map(({ value, label }) => (
          <button
            key={value}
            className={`${styles.controlButton} ${state.speed === value ? styles.controlButtonActive : ''}`}
            onClick={() => onSetSpeed(value)}
          >
            {label}
          </button>
        ))}
      </div>

      <input
        type="range"
        className={styles.scrubber}
        min={0}
        max={state.duration}
        value={state.time}
        onChange={e => onSeek(Number(e.target.value))}
      />

      <span className={styles.timeDisplay}>
        {formatTime(state.time)} / {formatTime(state.duration)}
      </span>

      <div className={styles.legend}>
        {LEGEND_ITEMS.map(item => (
          <span key={item.label} className={styles.legendItem}>
            <span className={styles.legendDot} style={{ backgroundColor: item.color }} />
            {item.label}
          </span>
        ))}
      </div>
    </div>
  );
}
