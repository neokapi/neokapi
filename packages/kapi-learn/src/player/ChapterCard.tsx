import React from "react";
import {
  ChevronLeft,
  ChevronRight,
  FileText,
  RotateCcw,
  Terminal as TerminalIcon,
} from "lucide-react";
import type { Chapter } from "../curriculum/types.ts";
import type { UpNext } from "./types.ts";

// The card under the stage: which chapter this is, what it does, and the two
// controls that move the lab, Previous and Next. Next names the chapter it
// goes to; in a terminal lab that chapter's command runs when it is pressed,
// so nothing happens on a timer.

export interface ChapterCardProps {
  chapter: Chapter;
  index: number;
  total: number;
  /** The chapter has run (a terminal lab) or been shown (an explorer lab). */
  ran: boolean;
  /** The exit code its command returned, when it has run. */
  exitCode: number | null;
  ready: boolean;
  busy: boolean;
  /** The chapter Next goes to, when there is one. */
  nextChapter?: Chapter;
  /** Every chapter has run or been shown. */
  atEnd: boolean;
  upNext?: UpNext;
  onPrevious: () => void;
  onNext: () => void;
  /** Start the lab over (terminal labs). */
  onReplay?: () => void;
  onOpenFile?: (path: string) => void;
}

/** The narration with the words to notice set in bold. */
function withNotice(text: string, notice: string[] | undefined): React.ReactNode {
  if (!notice || notice.length === 0) return text;
  const escaped = notice.map((n) => n.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"));
  const re = new RegExp(`(${escaped.join("|")})`, "g");
  const parts = text.split(re);
  return parts.map((part, i) =>
    notice.includes(part) ? (
      <strong key={i} className="kl-notice">
        {part}
      </strong>
    ) : (
      <React.Fragment key={i}>{part}</React.Fragment>
    ),
  );
}

/** What to say about the command's exit, before and after it runs. */
function exitNote(chapter: Chapter, ran: boolean, exitCode: number | null): string | null {
  const expected = chapter.exit ?? 0;
  if (ran && exitCode !== null) {
    if (exitCode === 0) return null;
    return exitCode === expected ? `exited ${exitCode}, as expected` : `exited ${exitCode}`;
  }
  return expected === 0 ? null : `expected to exit ${expected}`;
}

export default function ChapterCard({
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
  onNext,
  onReplay,
  onOpenFile,
}: ChapterCardProps): React.ReactElement {
  const note = exitNote(chapter, ran, exitCode);
  return (
    <section className="kl-step" aria-live="polite" aria-label={`Chapter ${index + 1}`}>
      <p className="kl-step__pos">
        Chapter {index + 1} of {total}
      </p>
      <h2 className="kl-step__title">{chapter.title}</h2>
      <p className="kl-step__text">{withNotice(chapter.narration, chapter.notice)}</p>
      {chapter.command && (
        <p className="kl-step__cmd">
          <TerminalIcon size={14} aria-hidden="true" />
          <code>{chapter.command}</code>
          {note && <span className="kl-step__exit">{note}</span>}
        </p>
      )}
      {chapter.note && <p className="kl-step__note">{chapter.note}</p>}
      {chapter.look?.file && onOpenFile && (
        <button
          type="button"
          className="kl-linkbtn"
          onClick={() => onOpenFile(chapter.look!.file!)}
        >
          <FileText size={14} aria-hidden="true" />
          Open {chapter.look.file}
        </button>
      )}

      <div className="kl-step__nav">
        <button
          type="button"
          className="kl-stepbtn"
          onClick={onPrevious}
          disabled={!ready || busy || index === 0}
        >
          <ChevronLeft size={18} aria-hidden="true" />
          Previous
        </button>
        {!atEnd && nextChapter ? (
          <button
            type="button"
            className="kl-stepbtn kl-stepbtn--next"
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
          <a className="kl-stepbtn kl-stepbtn--next" href={upNext.href}>
            <span className="kl-stepbtn__label">
              <span className="kl-stepbtn__small">Next lab</span>
              <span className="kl-stepbtn__title">{upNext.title}</span>
            </span>
            <ChevronRight size={18} aria-hidden="true" />
          </a>
        ) : atEnd && onReplay ? (
          <button
            type="button"
            className="kl-stepbtn kl-stepbtn--next"
            onClick={onReplay}
            disabled={!ready || busy}
          >
            <RotateCcw size={16} aria-hidden="true" />
            Start over
          </button>
        ) : null}
      </div>
      {atEnd && (
        <p className="kl-step__done">
          Every chapter has run. The terminal is yours: the list under the player has more to try.
        </p>
      )}
    </section>
  );
}
