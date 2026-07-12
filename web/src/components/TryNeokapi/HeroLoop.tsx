import React, { useEffect, useState } from "react";
import clsx from "clsx";
import { ArrowDown, ArrowRight, Check, ChevronRight, CornerLeftUp } from "lucide-react";
import styles from "./HeroLoop.module.css";

// The hero: an animated "content loop" — two coupled, repeating cycles that
// convey convergence (the loop never ends = no toil; you don't re-run a
// pipeline, the loop keeps content converged). Loop 1 is the single-language
// content cycle (Shape → Write → Check → Ship, repeat); loop 2 is the same
// cycle run per language (Read → Prep → Recycle → Translate → Check → Ship,
// repeat × every language). The content loop's Ship flows into the multilingual
// Read — the loops are coupled by a "× every language" bridge.
//
// Motion is convergence: a single active highlight travels each row of verb
// pills left→right, then the row's return arc pulses, then it loops — calm and
// continuous, paused on hover/focus. Idle pills show just the verb; the active
// pill reveals its sub-label, so two full loops stay uncrowded at hero width.
//
// Pure SVG/CSS/JS — nothing here boots the wasm engine. The whole card is a
// button that opens the live modal (which does boot wasm, unchanged). Under
// prefers-reduced-motion the loops sit at rest: no travel, no dash march.

interface HeroLoopProps {
  /** Open the full modal. */
  onOpen: () => void;
}

interface Step {
  verb: string;
  sub: string;
  /** A gate step (Ship) carries a small "✓" affordance when active. */
  gate?: boolean;
}

// Loop 1 — the content loop, one language (single-player content quality).
const CONTENT_LOOP: Step[] = [
  { verb: "Shape", sub: "terms · brand · tone" },
  { verb: "Write", sub: "you or your AI" },
  { verb: "Check", sub: "lint · terms · human review" },
  { verb: "Ship", sub: "on brand", gate: true },
];

// The tight inner iteration inside loop 1: a short back-arrow from Check to
// Write. Its index is the position of the pill the back-arrow returns TO (Write).
const INNER_RETURN_AT = 1;

// Loop 2 — the same loop, run per language.
const MULTILINGUAL_LOOP: Step[] = [
  { verb: "Read", sub: "any format, in English" },
  { verb: "Prep", sub: "protect terms · redact" },
  { verb: "Recycle", sub: "reuse what's translated" },
  { verb: "Translate", sub: "your AI" },
  { verb: "Check", sub: "red-line · green-line" },
  { verb: "Ship", sub: "every language", gate: true },
];

// Time (ms) the pulse dwells on each pill / on the return arc.
const STEP_MS = 700;

function usePrefersReducedMotion(): boolean {
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

// One loop = a row of verb pills joined by chevrons, closed by a sweeping return
// arc. The traveling pulse is an index into a virtual sequence [0..n] where the
// final slot (n) is the return arc.
function Loop({
  title,
  sub,
  steps,
  active,
  reduced,
  arcLabel,
  innerReturnAt,
}: {
  title: string;
  sub: string;
  steps: Step[];
  /** Active slot: 0..steps.length-1 lights a pill; steps.length lights the arc. */
  active: number;
  reduced: boolean;
  arcLabel: string;
  innerReturnAt?: number;
}): React.ReactElement {
  const n = steps.length;
  const arcActive = active === n;
  return (
    <div className={styles.loop}>
      <div className={styles.loopHead}>
        <span className={styles.loopTitle}>{title}</span>
        <span className={styles.loopSub}>{sub}</span>
      </div>

      <div className={styles.row}>
        {steps.map((step, i) => {
          const isActive = i === active;
          return (
            <React.Fragment key={step.verb + i}>
              <span className={clsx(styles.pill, isActive && styles.pillActive)}>
                <span className={styles.pillVerb}>
                  {step.verb}
                  {step.gate && isActive && (
                    <span className={styles.gate} aria-hidden="true">
                      <Check size={12} strokeWidth={3} />
                    </span>
                  )}
                </span>
                <span className={styles.pillSub}>{step.sub}</span>
              </span>
              {i < n - 1 && (
                <span
                  className={clsx(styles.chevron, active === i && styles.chevronLit)}
                  aria-hidden="true"
                >
                  <ChevronRight size={14} />
                </span>
              )}
            </React.Fragment>
          );
        })}
      </div>

      {/* The tight inner iteration (Write ⇄ Check): a short back-arrow. */}
      {innerReturnAt !== undefined && (
        <div className={styles.innerReturn} aria-hidden="true">
          <CornerLeftUp size={12} />
          <span>refine</span>
        </div>
      )}

      {/* The return arc: a full-width SVG curve from the last pill back to the
          first, with marching dashes and a "repeat" label. Decorative. */}
      <div className={styles.arcWrap}>
        <svg
          className={styles.arc}
          viewBox="0 0 100 20"
          preserveAspectRatio="none"
          aria-hidden="true"
        >
          <path
            className={clsx(
              styles.arcPath,
              !reduced && styles.arcFlow,
              arcActive && styles.arcActive,
            )}
            d="M 98 1 C 98 16, 80 18, 50 18 C 20 18, 2 16, 2 1"
          />
        </svg>
        <span className={clsx(styles.arcLabel, arcActive && styles.arcLabelActive)}>{arcLabel}</span>
      </div>
    </div>
  );
}

export default function HeroLoop({ onOpen }: HeroLoopProps): React.ReactElement {
  const reduced = usePrefersReducedMotion();
  const [paused, setPaused] = useState(false);

  // One continuous pulse traverses both loops in sequence: content-loop pills,
  // its return arc, then multilingual-loop pills, its return arc, then repeat.
  // Slots: [0..C] for loop 1 (C = arc), then [C+1..C+1+M] for loop 2.
  const C = CONTENT_LOOP.length; // arc slot for loop 1
  const M = MULTILINGUAL_LOOP.length; // arc slot offset for loop 2
  const TOTAL = C + 1 + M + 1;
  const [tick, setTick] = useState(0);

  useEffect(() => {
    if (reduced || paused) return;
    const t = setInterval(() => setTick((p) => (p + 1) % TOTAL), STEP_MS);
    return () => clearInterval(t);
  }, [reduced, paused, TOTAL]);

  // Under reduced motion there is no traveling pulse — the loops sit at rest
  // (all pills idle, arcs static). Passing -1 lights nothing.
  const pulse = reduced ? -1 : tick;
  const contentActive = pulse <= C ? pulse : -1;
  const multiActive = pulse > C ? pulse - (C + 1) : -1;

  return (
    <button
      type="button"
      className={styles.card}
      onClick={onOpen}
      onMouseEnter={() => setPaused(true)}
      onMouseLeave={() => setPaused(false)}
      onFocus={() => setPaused(true)}
      onBlur={() => setPaused(false)}
      aria-label="Open the interactive Kapi showcase — content flows around a loop that repeats until it converges"
    >
      <span className={styles.srOnly}>
        The content loop, one language: Shape, Write, Check, Ship — then repeat. Going multilingual
        runs the same loop per language: Read, Prep, Recycle, Translate, Check, Ship — then repeat
        for every language. The loop never ends; content stays converged.
      </span>

      <Loop
        title="The content loop · one language"
        sub="shape once, keep it on brand"
        steps={CONTENT_LOOP}
        active={contentActive}
        reduced={reduced}
        arcLabel="repeat"
        innerReturnAt={INNER_RETURN_AT}
      />

      {/* The loops are coupled: the content loop's Ship flows down into the
          multilingual Read, run once per language. */}
      <div className={styles.bridge} aria-hidden="true">
        <ArrowDown size={13} className={styles.bridgeIcon} />
        <span>× every language</span>
      </div>

      <Loop
        title="Going multilingual"
        sub="the same loop, per language"
        steps={MULTILINGUAL_LOOP}
        active={multiActive}
        reduced={reduced}
        arcLabel="repeat × every language"
      />

      <div className={styles.ctaRow}>
        <span className={styles.cta}>
          Try Kapi in your browser <ArrowRight size={16} aria-hidden="true" />
        </span>
      </div>
    </button>
  );
}
