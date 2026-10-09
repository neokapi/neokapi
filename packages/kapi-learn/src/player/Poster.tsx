import React from "react";
import { Play, RotateCcw } from "lucide-react";
import type { BootProgress } from "@neokapi/kapi-playground/runtime";
import type { Lab, SampleInfo } from "../curriculum/types.ts";
import type { SessionStatus } from "./types.ts";

// The poster over the stage until the engine is up: what the lab is, how long
// it takes, and one Play. Nothing downloads before it is pressed; while it
// loads, the poster shows the download the way a player shows buffering.

export interface PosterProps {
  lab: Lab;
  sample: SampleInfo;
  seriesTitle: string;
  status: SessionStatus;
  bootProgress: BootProgress | null;
  error: string | null;
  /** The engine is already up from another lab: Play needs no download. */
  warm: boolean;
  onStart: () => void;
  /** What Play loads, in a sentence (default: the engine). */
  hint?: string;
  /** What is loading right now, as a label beside the progress bar. */
  loadingLabel?: string;
}

function mb(bytes: number): string {
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

export default function Poster({
  lab,
  sample,
  seriesTitle,
  status,
  bootProgress,
  error,
  warm,
  onStart,
  hint,
  loadingLabel,
}: PosterProps): React.ReactElement {
  const loading = status === "booting" || status === "seeding";
  const frac =
    bootProgress && bootProgress.total
      ? Math.min(1, bootProgress.loaded / bootProgress.total)
      : null;
  const commands = lab.chapters.filter((c) => c.command).length;

  return (
    <div className={`kl-poster kl-poster--${lab.sample}`} data-status={status}>
      <div className="kl-poster__inner">
        <div className="kl-poster__eyebrow">
          <span>{seriesTitle}</span>
          <span aria-hidden="true">·</span>
          <span>{sample.name}</span>
        </div>
        <h2 className="kl-poster__title">{lab.title}</h2>
        <p className="kl-poster__tagline">{lab.tagline}</p>
        <p className="kl-poster__meta">
          {lab.chapters.length} chapters
          {commands > 0 ? ` · ${commands} commands` : ""} · about {lab.minutes} min
        </p>

        {status === "error" ? (
          <div className="kl-poster__error" role="alert">
            <p>The lab could not start: {error}</p>
            <button type="button" className="kl-btn kl-btn--primary" onClick={onStart}>
              <RotateCcw size={16} aria-hidden="true" />
              Try again
            </button>
          </div>
        ) : loading ? (
          <div className="kl-poster__loading" role="status" aria-live="polite">
            <div className="kl-progress" aria-hidden="true">
              <div
                className={`kl-progress__fill${frac === null ? " kl-progress__fill--indeterminate" : ""}`}
                style={frac === null ? undefined : { width: `${Math.round(frac * 100)}%` }}
              />
            </div>
            <p>
              {status === "seeding"
                ? `Opening the ${sample.name} sample…`
                : loadingLabel
                  ? `${loadingLabel}${frac !== null && bootProgress ? ` · ${mb(bootProgress.loaded)} of ${mb(bootProgress.total ?? 0)}` : "…"}`
                  : frac !== null && bootProgress
                    ? `Downloading the kapi engine · ${mb(bootProgress.loaded)} of ${mb(bootProgress.total ?? 0)}`
                    : warm
                      ? "Starting the engine…"
                      : "Starting the kapi engine…"}
            </p>
          </div>
        ) : (
          <div className="kl-poster__actions">
            <button
              type="button"
              className="kl-play"
              onClick={onStart}
              aria-label={`Play: start the lab "${lab.title}" in your browser`}
            >
              <Play size={30} aria-hidden="true" fill="currentColor" />
            </button>
            <p className="kl-poster__hint">
              {warm
                ? "The engine is already running in this tab."
                : (hint ??
                  "Runs the real kapi engine in your browser: a 20 MB download, once, kept for this session. Nothing leaves your machine.")}
            </p>
          </div>
        )}
      </div>
    </div>
  );
}
