import { useEffect, useRef, useState } from "react";

// useTextTransition — drive a source→target text change as one of three effects:
//
//   • "none"       — instant swap (also used when prefers-reduced-motion is set).
//   • "crossfade"  — instant value, but a changing `cycle` key lets the renderer
//                    re-mount + CSS-fade the new text (the renderer owns the CSS).
//   • "typewriter" — reveal the active text progressively (word- or char-by-word)
//                    so the target appears to be typed out.
//
// The hook is content-agnostic: it takes the *full* active text and returns the
// portion to render right now plus a `done` flag and a `cycle` counter. The
// caller maps the visible text to segments/overlays, so highlights appear as the
// words are revealed.

export type TransitionEffect = "none" | "crossfade" | "typewriter";
export type TypewriterGranularity = "word" | "char";

export interface UseTextTransitionOptions {
  effect: TransitionEffect;
  /** Typewriter granularity (default "word"). */
  granularity?: TypewriterGranularity;
  /** Reveal interval in ms per unit (default 28ms). */
  speed?: number;
  /**
   * Hold the text blank for this many ms before the typewriter starts — used to
   * stagger lines so they reveal one after another (default 0 = immediate).
   */
  delay?: number;
  /** Force reduced-motion (instant) regardless of media query — for tests. */
  reducedMotion?: boolean;
}

export interface TextTransitionState {
  /** The text to render right now (a growing prefix for typewriter). */
  visible: string;
  /** True once the full text is shown. */
  done: boolean;
  /** Increments each time the active text changes (a crossfade re-mount key). */
  cycle: number;
}

function prefersReducedMotion(): boolean {
  if (typeof window === "undefined" || !window.matchMedia) return false;
  return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}

/** Split text into reveal units (whole words + their trailing space, or chars). */
function splitUnits(text: string, granularity: TypewriterGranularity): string[] {
  if (granularity === "char") return Array.from(text);
  // Group each word with the whitespace that follows it so spacing is preserved.
  return text.match(/\S+\s*|\s+/g) ?? [];
}

export function useTextTransition(
  text: string,
  opts: UseTextTransitionOptions,
): TextTransitionState {
  const { effect, granularity = "word", speed = 28, delay = 0, reducedMotion } = opts;
  const reduce = reducedMotion ?? prefersReducedMotion();

  const [visible, setVisible] = useState(text);
  const [done, setDone] = useState(true);
  const cycleRef = useRef(0);
  const [cycle, setCycle] = useState(0);
  const prevText = useRef(text);

  useEffect(() => {
    const changed = prevText.current !== text;
    prevText.current = text;
    if (changed) {
      cycleRef.current += 1;
      setCycle(cycleRef.current);
    }

    // Instant paths: no effect, reduced motion, crossfade (CSS owns the fade), or
    // an unchanged value.
    if (effect !== "typewriter" || reduce || !changed) {
      setVisible(text);
      setDone(true);
      return;
    }

    // Typewriter: reveal unit by unit, after an optional stagger delay (during
    // which the line stays blank so lines reveal one after another).
    const units = splitUnits(text, granularity);
    let i = 0;
    setVisible("");
    setDone(units.length === 0);
    let acc = "";
    let interval: ReturnType<typeof setInterval> | undefined;
    const startTyping = () => {
      interval = setInterval(
        () => {
          acc += units[i] ?? "";
          i += 1;
          setVisible(acc);
          if (i >= units.length) {
            setDone(true);
            if (interval) clearInterval(interval);
          }
        },
        Math.max(8, speed),
      );
    };
    const startTimer = delay > 0 ? setTimeout(startTyping, delay) : undefined;
    if (!startTimer) startTyping();
    return () => {
      if (startTimer) clearTimeout(startTimer);
      if (interval) clearInterval(interval);
    };
  }, [text, effect, granularity, speed, delay, reduce]);

  return { visible, done, cycle };
}
