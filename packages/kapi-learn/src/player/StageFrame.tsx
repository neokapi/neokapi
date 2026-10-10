import React, { useCallback, useEffect, useState } from "react";
import { Maximize2, Minimize2 } from "lucide-react";

// The frame around the stage: a slim bar naming what is on it, with Expand,
// and the stage itself under the bar. Expand takes the stage to the whole
// window, which a terminal needs: the page column fits eighty-odd characters
// and a kapi report wants more. Escape, or the same control, brings it back.
// The terminal inside refits itself to the new box. Until the stage has
// something on it the bar is absent and the poster covers the frame.

export interface StageFrameProps {
  children: React.ReactNode;
  /** Show the expand control (once the stage has something on it). */
  expandable?: boolean;
  className?: string;
  /** What the stage holds, as the bar names it ("Terminal", "Explorer"). */
  label?: string;
}

export default function StageFrame({
  children,
  expandable = false,
  className = "",
  label = "Stage",
}: StageFrameProps): React.ReactElement {
  const [expanded, setExpanded] = useState(false);
  const what = label.toLowerCase();
  const toggle = useCallback(() => setExpanded((v) => !v), []);

  useEffect(() => {
    if (!expanded) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setExpanded(false);
    };
    document.addEventListener("keydown", onKey);
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.removeEventListener("keydown", onKey);
      document.body.style.overflow = prev;
    };
  }, [expanded]);

  return (
    <>
      {expanded && <div className="kl-stage-placeholder" aria-hidden="true" />}
      <div
        className={`kl-stage${expanded ? " kl-stage--expanded" : ""} ${className}`.trim()}
        tabIndex={-1}
        role={expanded ? "dialog" : undefined}
        aria-modal={expanded ? true : undefined}
        aria-label={expanded ? `${what}, expanded` : undefined}
      >
        {expandable && (
          <div className="kl-stage__bar">
            <span className="kl-stage__bar-label">{label}</span>
            <button
              type="button"
              className="kl-stage__expand"
              onClick={toggle}
              aria-label={expanded ? `Shrink the ${what}` : `Expand the ${what} to the window`}
              title={expanded ? "Shrink (esc)" : "Expand"}
            >
              {expanded ? (
                <Minimize2 size={15} aria-hidden="true" />
              ) : (
                <Maximize2 size={15} aria-hidden="true" />
              )}
            </button>
          </div>
        )}
        <div className="kl-stage__inner">{children}</div>
      </div>
    </>
  );
}
