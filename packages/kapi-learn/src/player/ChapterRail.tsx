import React, { useEffect, useRef } from "react";
import { Check, ChevronRight, CircleDashed } from "lucide-react";
import type { Chapter } from "../curriculum/types.ts";

// The chapter list beside the stage, the way a video site lists what plays
// next: number, title, the command it runs, and whether it has run.

export interface ChapterRailProps {
  chapters: readonly Chapter[];
  current: number;
  executed: number;
  ready: boolean;
  onSelect: (index: number) => void;
}

export default function ChapterRail({
  chapters,
  current,
  executed,
  ready,
  onSelect,
}: ChapterRailProps): React.ReactElement {
  const listRef = useRef<HTMLOListElement>(null);

  // Keep the chapter in view visible as autoplay moves on.
  useEffect(() => {
    const item = listRef.current?.querySelector<HTMLElement>(`[data-index="${current}"]`);
    item?.scrollIntoView({ block: "nearest", behavior: "smooth" });
  }, [current]);

  return (
    <ol ref={listRef} className="kl-rail__list" aria-label="Chapters">
      {chapters.map((ch, i) => {
        const done = i < executed;
        const isCurrent = i === current;
        return (
          <li key={ch.id} data-index={i}>
            <button
              type="button"
              className={`kl-chapter${isCurrent ? " kl-chapter--current" : ""}${done ? " kl-chapter--done" : ""}`}
              onClick={() => onSelect(i)}
              disabled={!ready}
              aria-current={isCurrent ? "step" : undefined}
            >
              <span className="kl-chapter__num" aria-hidden="true">
                {done ? (
                  <Check size={14} />
                ) : isCurrent ? (
                  <ChevronRight size={14} />
                ) : (
                  <CircleDashed size={14} />
                )}
              </span>
              <span className="kl-chapter__body">
                <span className="kl-chapter__title">
                  <span className="kl-chapter__index">{i + 1}</span> {ch.title}
                </span>
                {ch.command ? (
                  <code className="kl-chapter__cmd" title={ch.command}>
                    {ch.command}
                  </code>
                ) : (
                  <span className="kl-chapter__cmd kl-chapter__cmd--read">read</span>
                )}
              </span>
              <span className="kl-chapter__sr">
                {done ? "done" : isCurrent ? "current" : "not yet run"}
              </span>
            </button>
          </li>
        );
      })}
    </ol>
  );
}
