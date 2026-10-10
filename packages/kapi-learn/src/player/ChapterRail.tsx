import React, { useEffect, useRef } from "react";
import { Check, ChevronRight, CircleDashed } from "lucide-react";
import type { Chapter } from "../curriculum/types.ts";

// The chapter list beside the stage: number, title, the command it runs (or
// what it shows), and whether it has run. Pressing one jumps there; the
// chapters before it run at once.

export interface ChapterRailProps {
  chapters: readonly Chapter[];
  current: number;
  /** How many chapters have run, from the first (a terminal lab). */
  executed: number;
  /** Chapters the reader has viewed (an explorer lab). */
  visited?: ReadonlySet<string>;
  ready: boolean;
  onSelect: (index: number) => void;
}

export default function ChapterRail({
  chapters,
  current,
  executed,
  visited,
  ready,
  onSelect,
}: ChapterRailProps): React.ReactElement {
  const listRef = useRef<HTMLOListElement>(null);

  // Keep the chapter in view visible as the reader steps on, scrolling the
  // list alone: scrollIntoView would scroll the page as well, and pull the
  // stage out of view just as the chapter changes it.
  useEffect(() => {
    const list = listRef.current;
    const item = list?.querySelector<HTMLElement>(`[data-index="${current}"]`);
    if (!list || !item) return;
    const top =
      item.getBoundingClientRect().top - list.getBoundingClientRect().top + list.scrollTop;
    const bottom = top + item.offsetHeight;
    if (top < list.scrollTop) list.scrollTo({ top, behavior: "smooth" });
    else if (bottom > list.scrollTop + list.clientHeight) {
      list.scrollTo({ top: bottom - list.clientHeight, behavior: "smooth" });
    }
  }, [current]);

  return (
    <ol ref={listRef} className="kl-rail__list" aria-label="Chapters">
      {chapters.map((ch, i) => {
        const done = i < executed || (visited?.has(ch.id) ?? false);
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
                ) : ch.hint ? (
                  <span className="kl-chapter__cmd kl-chapter__cmd--read">{ch.hint}</span>
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
