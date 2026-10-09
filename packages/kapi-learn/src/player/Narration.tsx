import React from "react";
import { FileText, Terminal as TerminalIcon } from "lucide-react";
import type { Chapter } from "../curriculum/types.ts";

// What a video site puts under the player: the chapter's title, its
// narration, what to notice in the output, and where to look.

export interface NarrationProps {
  chapter: Chapter;
  index: number;
  total: number;
  /** The chapter has run. */
  done: boolean;
  /** The exit code its command returned, when it has run. */
  exitCode: number | null;
  onOpenFile: (path: string) => void;
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

export default function Narration({
  chapter,
  index,
  total,
  done,
  exitCode,
  onOpenFile,
}: NarrationProps): React.ReactElement {
  const expected = chapter.exit ?? 0;
  return (
    <section className="kl-narration" aria-live="polite">
      <div className="kl-narration__eyebrow">
        <span>
          Chapter {index + 1} of {total}
        </span>
        {chapter.command && (
          <span className={`kl-exit kl-exit--${expected === 0 ? "ok" : "gate"}`}>
            {done && exitCode !== null ? `exit ${exitCode}` : `expects exit ${expected}`}
          </span>
        )}
      </div>
      <h2 className="kl-narration__title">{chapter.title}</h2>
      {chapter.command && (
        <p className="kl-narration__cmd">
          <TerminalIcon size={14} aria-hidden="true" />
          <code>{chapter.command}</code>
        </p>
      )}
      <p className="kl-narration__text">{withNotice(chapter.narration, chapter.notice)}</p>
      {chapter.notice && chapter.notice.length > 0 && (
        <p className="kl-narration__look">
          Notice{" "}
          {chapter.notice.map((n, i) => (
            <React.Fragment key={n}>
              {i > 0 && ", "}
              <code>{n}</code>
            </React.Fragment>
          ))}{" "}
          in the output.
        </p>
      )}
      {chapter.note && <p className="kl-narration__note">{chapter.note}</p>}
      {chapter.look?.file && (
        <button
          type="button"
          className="kl-linkbtn"
          onClick={() => onOpenFile(chapter.look!.file!)}
        >
          <FileText size={14} aria-hidden="true" />
          Open {chapter.look.file} in the files pane
        </button>
      )}
    </section>
  );
}
