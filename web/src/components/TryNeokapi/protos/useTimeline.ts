import { useEffect, useRef, useState } from "react";

// A small looping timeline for the animated hero prototypes: advance through
// `phases` on the given per-phase durations, pause on hover/focus, and honour
// prefers-reduced-motion (no auto-advance; parks on a chosen still phase).

export interface Timeline {
  phase: number;
  setPhase: (p: number) => void;
  paused: boolean;
  reduced: boolean;
  /** Spread onto the stage container to pause on hover/focus. */
  hoverBind: {
    onMouseEnter: () => void;
    onMouseLeave: () => void;
    onFocusCapture: () => void;
    onBlurCapture: () => void;
  };
}

export function useReducedMotion(): boolean {
  const [reduced, setReduced] = useState(false);
  useEffect(() => {
    const mq = window.matchMedia("(prefers-reduced-motion: reduce)");
    setReduced(mq.matches);
    const on = (e: MediaQueryListEvent) => setReduced(e.matches);
    mq.addEventListener("change", on);
    return () => mq.removeEventListener("change", on);
  }, []);
  return reduced;
}

export function useTimeline(
  phases: number,
  durationMs: number | number[],
  opts?: { reducedStill?: number },
): Timeline {
  const reduced = useReducedMotion();
  const reducedStill = opts?.reducedStill ?? phases - 1;
  const [phase, setPhase] = useState(0);
  const [paused, setPaused] = useState(false);
  const manual = useRef(false);

  // Park on the still phase under reduced motion.
  useEffect(() => {
    if (reduced) setPhase(reducedStill);
  }, [reduced, reducedStill]);

  useEffect(() => {
    if (reduced || paused) return;
    const dur = Array.isArray(durationMs) ? (durationMs[phase] ?? 2600) : durationMs;
    const t = setTimeout(() => {
      manual.current = false;
      setPhase((p) => (p + 1) % phases);
    }, dur);
    return () => clearTimeout(t);
  }, [phase, paused, reduced, phases, durationMs]);

  return {
    phase,
    setPhase: (p: number) => {
      manual.current = true;
      setPhase(p);
    },
    paused,
    reduced,
    hoverBind: {
      onMouseEnter: () => setPaused(true),
      onMouseLeave: () => setPaused(false),
      onFocusCapture: () => setPaused(true),
      onBlurCapture: () => setPaused(false),
    },
  };
}
