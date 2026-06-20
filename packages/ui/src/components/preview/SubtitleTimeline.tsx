import React, { useMemo, useState } from "react";
import { cn } from "../../lib/utils";
import { Badge } from "../ui/badge";
import { ToggleGroup, ToggleGroupItem } from "../ui/toggle-group";
import { collectCues, formatDuration, formatTimecode, type Cue } from "./timeline";
import { runsText } from "./renderDoc";
import type { ContentTree } from "./types";

// SubtitleTimeline — a pure, presentational cue list for time-based sources
// (SRT/VTT/TTML, audio/video transcripts). It lifts every block carrying a
// `timing` anchor into a start-ordered list and shows, per cue: its timecode
// (HH:MM:SS.mmm), the source runs, and — when a target locale is selected — the
// target runs, with a source↔target toggle. It owns no media element: a parent
// drives the active-cue highlight with `currentTimeMs` and is told which cue the
// reader clicked via `onSeek(ms)`, so the same timeline serves the standalone
// view, the AudioPlayer, and the VideoPlayer.

export interface SubtitleTimelineProps {
  /** The engine output; cues are the blocks carrying a `timing` anchor. */
  tree: ContentTree;
  /**
   * Target variant key to show in the target column ("source" = source only).
   * Uncontrolled by default: when omitted and targets exist, the first locale is
   * preselected and a toggle lets the reader switch.
   */
  side?: string;
  /** Called when `side` should change (controlled mode). */
  onSideChange?: (side: string) => void;
  /** Current playhead in ms — the cue spanning it is highlighted. */
  currentTimeMs?: number;
  /** Clicking a cue calls this with the cue's start time (ms). */
  onSeek?: (ms: number) => void;
  className?: string;
}

/** The distinct target locales present across the cues, in first-seen order. */
function localesOf(cues: Cue[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const c of cues) {
    for (const k of Object.keys(c.node.targets ?? {})) {
      if (!seen.has(k)) {
        seen.add(k);
        out.push(k);
      }
    }
  }
  return out;
}

function targetText(cue: Cue, locale: string): string {
  const runs = cue.node.targets?.[locale];
  return runs && runs.length > 0 ? runsText(runs) : "";
}

export default function SubtitleTimeline({
  tree,
  side,
  onSideChange,
  currentTimeMs,
  onSeek,
  className,
}: SubtitleTimelineProps): React.ReactElement {
  const cues = useMemo(() => collectCues(tree), [tree]);
  const locales = useMemo(() => localesOf(cues), [cues]);

  // Active target column: controlled by `side`, else the first locale (uncontrolled).
  const [internalSide, setInternalSide] = useState<string>(() => side ?? locales[0] ?? "source");
  const activeSide = side ?? internalSide;
  const setSide = (v: string) => {
    if (!v) return;
    onSideChange?.(v);
    if (side === undefined) setInternalSide(v);
  };
  const showTarget = activeSide !== "source" && locales.includes(activeSide);

  if (cues.length === 0) {
    return (
      <p className={cn("py-3 text-sm text-muted-foreground", className)}>
        No timed cues in this document — the subtitle timeline applies to time-based sources
        (subtitles, audio/video transcripts).
      </p>
    );
  }

  return (
    <div className={cn("flex flex-col gap-2", className)}>
      {locales.length > 0 && (
        <ToggleGroup
          type="single"
          size="sm"
          value={activeSide}
          onValueChange={setSide}
          aria-label="Source or target"
          className="w-fit"
        >
          <ToggleGroupItem value="source">Source</ToggleGroupItem>
          {locales.map((loc) => (
            <ToggleGroupItem key={loc} value={loc}>
              {loc}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
      )}

      <ol className="flex flex-col divide-y divide-border/40" data-testid="subtitle-timeline">
        {cues.map((cue) => {
          const active =
            currentTimeMs !== undefined &&
            currentTimeMs >= cue.timing.startMs &&
            currentTimeMs < cue.timing.endMs;
          const source = runsText(cue.node.source);
          const target = showTarget ? targetText(cue, activeSide) : "";
          return (
            <li key={cue.node.id}>
              <button
                type="button"
                onClick={() => onSeek?.(cue.timing.startMs)}
                data-testid="cue-row"
                data-cue-index={cue.index}
                data-active={active ? "true" : "false"}
                className={cn(
                  "flex w-full items-start gap-3 rounded-md px-2 py-1.5 text-left transition-colors hover:bg-muted/50",
                  active && "bg-primary/10 ring-1 ring-primary/40",
                )}
              >
                <span className="flex shrink-0 flex-col items-start gap-0.5">
                  <span className="font-mono text-[11px] tabular-nums text-muted-foreground">
                    {formatTimecode(cue.timing.startMs)}
                  </span>
                  <Badge variant="ghost" className="px-1 py-0 text-[9px] text-muted-foreground">
                    {formatDuration(cue.timing.endMs - cue.timing.startMs)}
                  </Badge>
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block text-sm text-foreground" data-testid="cue-source">
                    {source || <span className="italic text-muted-foreground">(empty)</span>}
                  </span>
                  {showTarget && (
                    <span
                      className="mt-0.5 block text-sm text-primary/90"
                      data-testid="cue-target"
                      lang={activeSide}
                    >
                      {target || (
                        <span className="italic text-muted-foreground/70">(untranslated)</span>
                      )}
                    </span>
                  )}
                </span>
              </button>
            </li>
          );
        })}
      </ol>
    </div>
  );
}
