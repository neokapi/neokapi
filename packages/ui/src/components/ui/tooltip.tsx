import * as React from "react";
import { useState, useRef, useEffect, useCallback } from "react";
import { cn } from "../../lib/utils";

interface TooltipProps {
  children: React.ReactElement;
  content: React.ReactNode;
  side?: "top" | "bottom" | "left" | "right";
  delayMs?: number;
}

function Tooltip({ children, content, side = "top", delayMs = 300 }: TooltipProps) {
  const [open, setOpen] = useState(false);
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const show = useCallback(() => {
    timeoutRef.current = setTimeout(() => setOpen(true), delayMs);
  }, [delayMs]);

  const hide = useCallback(() => {
    if (timeoutRef.current) clearTimeout(timeoutRef.current);
    setOpen(false);
  }, []);

  useEffect(() => {
    return () => {
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
    };
  }, []);

  const sideClasses = {
    top: "bottom-full left-1/2 -translate-x-1/2 mb-2",
    bottom: "top-full left-1/2 -translate-x-1/2 mt-2",
    left: "right-full top-1/2 -translate-y-1/2 mr-2",
    right: "left-full top-1/2 -translate-y-1/2 ml-2",
  };

  return (
    <div className="relative inline-flex" onMouseEnter={show} onMouseLeave={hide}>
      {children}
      {open && (
        <div
          className={cn(
            "absolute z-50 rounded-md bg-popover px-3 py-1.5 text-xs text-popover-foreground shadow-md border border-border animate-in fade-in-0 whitespace-nowrap",
            sideClasses[side],
          )}
          role="tooltip"
        >
          {content}
        </div>
      )}
    </div>
  );
}

export { Tooltip };
