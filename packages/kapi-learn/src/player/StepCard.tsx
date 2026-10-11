import React from "react";
import {
  ChevronLeft,
  ChevronRight,
  FileText,
  Play,
  RotateCcw,
  Terminal as TerminalIcon,
} from "lucide-react";
import type { Chapter } from "../curriculum/types.ts";
import { exitNote, withNotice } from "./ChapterCard.tsx";
import type { UpNext } from "./types.ts";

// The tour layout's card, beside the stage: the step in view, what it does,
// and the one control that runs it. Before the step runs the card shows the
// command Run will type; after, it adds what the output shows and offers
// Next. Back views the step before and runs nothing.

export interface StepCardProps {
  chapter: Chapter;
  index: number;
  total: number;
  /** The step has run. */
  ran: boolean;
  /** The exit code its command returned, when it has run. */
  exitCode: number | null;
  ready: boolean;
  busy: boolean;
  /** The step after this one, when there is one. */
  nextChapter?: Chapter;
  /** Every step has run. */
  atEnd: boolean;
  upNext?: UpNext;
  onPrevious: () => void;
  onRun: () => void;
  onNext: () => void;
  onReplay: () => void;
  onOpenFile?: (path: string) => void;
  /** The card takes focus when a step action leaves none, so the keys keep working. */
  ref?: React.Ref<HTMLElement>;
}

/** What Run does for this step, as its label. */
function runLabel(chapter: Chapter): string {
  if (chapter.command) return "Run this step";
  if (chapter.files?.length) return "Write the files";
  return "Mark as read";
}

export default function StepCard({
  chapter,
  index,
  total,
  ran,
  exitCode,
  ready,
  busy,
  nextChapter,
  atEnd,
  upNext,
  onPrevious,
  onRun,
  onNext,
  onReplay,
  onOpenFile,
  ref,
}: StepCardProps): React.ReactElement {
  const note = exitNote(chapter, ran, exitCode);
  const files = chapter.files ?? [];
  return (
    <section
      ref={ref}
      tabIndex={-1}
      className="klt-card"
      aria-live="polite"
      aria-label={`Step ${index + 1}`}
    >
      <p className="klt-card__head">
        <span>
          Step {index + 1} of {total}
        </span>
        <span className={`klt-card__state${ran ? " klt-card__state--ran" : ""}`}>
          {ran ? "ran" : "not yet run"}
        </span>
      </p>
      <h2 className="klt-card__title">{chapter.title}</h2>
      <p className="klt-card__text">{withNotice(chapter.narration, chapter.notice)}</p>
      {files.length > 0 && (
        <p className="klt-card__files">
          <FileText size={14} aria-hidden="true" />
          <span>
            {ran ? "Wrote" : "Writes"} {files.map((f) => f.path).join(", ")}
            {ran ? "." : " before the command runs."}
          </span>
        </p>
      )}
      {chapter.command && (
        <pre className="klt-card__cmd">
          <TerminalIcon size={14} aria-hidden="true" />
          <code>{chapter.command}</code>
        </pre>
      )}
      {note && <p className="klt-card__exit">{note}</p>}
      {ran && chapter.note && <p className="klt-card__note">{chapter.note}</p>}
      {ran && chapter.look?.file && onOpenFile && (
        <button
          type="button"
          className="kl-linkbtn"
          onClick={() => onOpenFile(chapter.look!.file!)}
        >
          <FileText size={14} aria-hidden="true" />
          Open {chapter.look.file}
        </button>
      )}

      <div className="klt-card__actions">
        <button
          type="button"
          className="kl-stepbtn"
          onClick={onPrevious}
          disabled={!ready || busy || index === 0}
          aria-label="Previous step"
        >
          <ChevronLeft size={18} aria-hidden="true" />
          Back
        </button>
        {!ran ? (
          <button
            type="button"
            className="kl-stepbtn klt-run"
            onClick={onRun}
            disabled={!ready || busy}
          >
            <Play size={16} aria-hidden="true" fill="currentColor" />
            {runLabel(chapter)}
          </button>
        ) : nextChapter ? (
          <button
            type="button"
            className="kl-stepbtn kl-stepbtn--next klt-next"
            onClick={onNext}
            disabled={!ready || busy}
          >
            <span className="kl-stepbtn__label">
              <span className="kl-stepbtn__small">Next</span>
              <span className="kl-stepbtn__title">{nextChapter.title}</span>
            </span>
            <ChevronRight size={18} aria-hidden="true" />
          </button>
        ) : atEnd && upNext ? (
          <a className="kl-stepbtn kl-stepbtn--next klt-next" href={upNext.href}>
            <span className="kl-stepbtn__label">
              <span className="kl-stepbtn__small">Next lab</span>
              <span className="kl-stepbtn__title">{upNext.title}</span>
            </span>
            <ChevronRight size={18} aria-hidden="true" />
          </a>
        ) : (
          <button
            type="button"
            className="kl-stepbtn kl-stepbtn--next klt-next"
            onClick={onReplay}
            disabled={!ready || busy}
          >
            <RotateCcw size={16} aria-hidden="true" />
            Start over
          </button>
        )}
      </div>
      {atEnd && (
        <p className="klt-card__done">
          Every step has run. The terminal is yours, and the list under it has more to try.
        </p>
      )}
    </section>
  );
}
