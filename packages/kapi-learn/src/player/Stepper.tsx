import React, { useEffect, useRef } from "react";
import { Check } from "lucide-react";
import type { Chapter } from "../curriculum/types.ts";

// The tour layout's stepper: one chip per step in a row across the top of the
// page, which scrolls sideways when the lab has more steps than the row fits.
// The chip set apart is the step whose card is beside the stage; a tick marks
// a step that has run, and the filled number is the one to run next. Pressing
// a chip views that step; the unrun steps before it are replayed at once.

export interface StepperProps {
  chapters: readonly Chapter[];
  current: number;
  /** How many steps have run, from the first. */
  executed: number;
  ready: boolean;
  busy: boolean;
  onSelect: (index: number) => void;
}

export default function Stepper({
  chapters,
  current,
  executed,
  ready,
  busy,
  onSelect,
}: StepperProps): React.ReactElement {
  const rowRef = useRef<HTMLOListElement>(null);

  // Keep the current chip in view, scrolling the row alone: scrollIntoView
  // would move the page as well.
  useEffect(() => {
    const row = rowRef.current;
    const item = row?.querySelector<HTMLElement>(`[data-index="${current}"]`);
    if (!row || !item) return;
    const r = row.getBoundingClientRect();
    const c = item.getBoundingClientRect();
    if (c.left < r.left) row.scrollBy({ left: c.left - r.left - 24, behavior: "smooth" });
    else if (c.right > r.right) row.scrollBy({ left: c.right - r.right + 24, behavior: "smooth" });
  }, [current]);

  return (
    <nav className="klt-stepper" aria-label="Steps">
      <ol ref={rowRef} className="klt-stepper__row">
        {chapters.map((ch, i) => {
          const done = i < executed;
          const isCurrent = i === current;
          const up = i === executed;
          const cls = [
            "klt-step",
            isCurrent && "klt-step--current",
            done && "klt-step--done",
            up && "klt-step--up",
          ]
            .filter(Boolean)
            .join(" ");
          return (
            <li key={ch.id} data-index={i}>
              <button
                type="button"
                className={cls}
                onClick={() => onSelect(i)}
                disabled={!ready || busy}
                aria-current={isCurrent ? "step" : undefined}
                title={ch.command ?? ch.title}
              >
                <span className="klt-step__num" aria-hidden="true">
                  {done ? <Check size={12} strokeWidth={3} /> : i + 1}
                </span>
                <span className="klt-step__title">{ch.title}</span>
                <span className="klt-step__sr">
                  {done ? "ran" : up ? "next to run" : "not yet run"}
                </span>
              </button>
            </li>
          );
        })}
      </ol>
    </nav>
  );
}
