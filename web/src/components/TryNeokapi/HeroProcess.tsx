import React, { useEffect, useState } from "react";
import clsx from "clsx";
import { FormatPreview } from "@neokapi/ui-primitives/preview";
import { ArrowRight, ChevronLeft, ChevronRight } from "lucide-react";
import { FRAMES, HERO_FILENAME, READ_FORMATS, STAGES } from "./heroStages";
import styles from "./styles.module.css";

// The hero: an auto-playing, six-stage "show" of kapi end to end —
// Read → Pre-process → Pseudo-translate → Leverage → Translate (ja) → Merge —
// rendered through the shared FormatPreview on BAKED data (heroStages.ts), so the
// page pulls ZERO wasm on load. The slide is an Acme pitch deck on ONE persistent
// card (on a peeking stack) — it never re-mounts, so prior results (redaction,
// terms, earlier text) stay put while each stage animates its own change in place.
//
// Each stage plays in two beats: it first HOLDS the previous stage's result (the
// "before" view) for ~2s, then animates into its own target ("after") view — so
// e.g. Pseudo-translate shows the source (already redacted, terms marked) for 2s,
// then the text rolls to the accented pseudo form. The card tints by the CURRENT
// stage's locale, so the background stays consistent for the whole stage and only
// changes at the boundary between stages. Prev/next arrows step through stages, a
// counter + caption name the stage, and a CTA opens the live modal.
//
// prefers-reduced-motion: no auto-advance, no hold, no roll; step with arrows.

interface HeroProcessProps {
  /** Open the full modal. */
  onOpen: () => void;
}

// How long each stage holds its "before" view (the previous stage's result)
// before animating into its own target view.
const HOLD = 2000;

// Per-stage time (ms) spent showing the animated "after" result before advancing.
// Stage 0 (Read) has no "before" to hold, so it simply shows for this long.
const AFTER = [2400, 2200, 2400, 2600, 2800, 2400];

// Stagger between line reveals on roll stages (ms per line index).
const LINE_STAGGER = 260;

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

export default function HeroProcess({ onOpen }: HeroProcessProps): React.ReactElement {
  const reduced = usePrefersReducedMotion();
  const [i, setI] = useState(0);
  // The stage index whose "after" view has been revealed. Derived `revealed`
  // compares it to the current stage, so a stage reads as NOT revealed on its very
  // first render (revealedFor still points at the previous stage) — no one-frame
  // flash of the target view, which would otherwise replay the before animation.
  const [revealedFor, setRevealedFor] = useState(0);
  const [paused, setPaused] = useState(false);
  const n = STAGES.length;

  // Stage 0 (Read) and reduced motion have no "before" hold; otherwise a stage is
  // revealed only once its hold timer has fired for it.
  const revealed = reduced || i === 0 || revealedFor === i;

  // The current stage names the chrome + caption and sets the card tint (constant
  // for the whole stage); the displayed CONTENT is the previous stage's result
  // while holding "before", then this stage's frame once revealed.
  const stage = STAGES[i];
  const shownStage = revealed ? stage : STAGES[(i - 1 + n) % n];
  const frame = FRAMES[shownStage.key];
  const stageLocale = FRAMES[stage.key].locale;

  const go = (delta: number) => setI((p) => (p + delta + n) % n);

  // Hold the "before" view for HOLD ms, then mark this stage revealed so it
  // animates into its target. Read/reduced motion need no timer (always revealed).
  useEffect(() => {
    if (reduced || i === 0) return;
    const t = setTimeout(() => setRevealedFor(i), HOLD);
    return () => clearTimeout(t);
  }, [i, reduced]);

  // Auto-advance, looping; paused on hover/focus and under reduced motion. Each
  // stage gets its "before" hold (HOLD, except Read) plus its "after" view time.
  useEffect(() => {
    if (reduced || paused) return;
    const dwell = i === 0 ? AFTER[0] : HOLD + AFTER[i];
    const t = setTimeout(() => setI((p) => (p + 1) % n), dwell);
    return () => clearTimeout(t);
  }, [i, reduced, paused, n]);

  return (
    <div
      className={styles.heroCard}
      onMouseEnter={() => setPaused(true)}
      onMouseLeave={() => setPaused(false)}
    >
      <div className={styles.heroStageWrap}>
        <button
          type="button"
          className={`${styles.heroArrow} ${styles.heroArrowPrev}`}
          onClick={() => go(-1)}
          aria-label="Previous stage"
        >
          <ChevronLeft size={18} />
        </button>

        {/* The card — the deck slide being processed, on a peeking stack. Opens the modal. */}
        <button
          type="button"
          className={styles.heroOpen}
          onClick={onOpen}
          aria-label="Open the interactive Try Neokapi showcase"
        >
          <div className={styles.heroChrome}>
            <span className={styles.heroStageName}>
              {i + 1}/{n} · {stage.label}
            </span>
          </div>

          {/* Stacked deck: two peeking back cards, then ONE persistent active
              card. The deck tints by the current stage's locale (en source → qps
              → ja target), constant for the whole stage. */}
          <div className={styles.deck} data-locale={stageLocale}>
            <span className={styles.deckBackFar} aria-hidden="true" />
            <span className={styles.deckBackNear} aria-hidden="true" />
            <div
              className={clsx(styles.heroStage, shownStage.key === "read" && styles.heroStageRead)}
            >
              <span className={styles.deckLabel} aria-hidden="true">
                <span className={styles.deckLabelLocale}>{stageLocale}</span>/{HERO_FILENAME}
              </span>
              <span className={styles.srOnly} aria-live="polite">
                {stage.label}: {stage.caption}
              </span>
              {frame.badge && (
                <span className={styles.heroBadge} key={`${shownStage.key}-badge`}>
                  {frame.badge}
                </span>
              )}
              <FormatPreview
                doc={frame.doc}
                side="source"
                annotations={frame.annotations}
                transition={reduced ? "none" : frame.transition}
                typewriterStagger={reduced ? 0 : LINE_STAGGER}
                reducedMotion={reduced}
                gridHeaders={false}
                flush
                className={styles.heroDoc}
              />
              {shownStage.key === "read" && (
                <div className={styles.heroFormats} aria-hidden="true">
                  <span className={styles.heroFormatsLabel}>reads</span>
                  {READ_FORMATS.map((f) => (
                    <span key={f} className={styles.heroFormatChip}>
                      {f}
                    </span>
                  ))}
                  <span className={`${styles.heroFormatChip} ${styles.heroFormatMore}`}>50+</span>
                </div>
              )}
            </div>
          </div>
        </button>

        <button
          type="button"
          className={`${styles.heroArrow} ${styles.heroArrowNext}`}
          onClick={() => go(1)}
          aria-label="Next stage"
        >
          <ChevronRight size={18} />
        </button>
      </div>

      <p className={styles.heroCaption} key={`${stage.key}-cap`}>
        <strong>{stage.label}.</strong> {stage.caption}
      </p>

      <button type="button" className={styles.heroCta} onClick={onOpen}>
        Try Kapi in your browser <ArrowRight size={16} aria-hidden="true" />
      </button>
    </div>
  );
}
