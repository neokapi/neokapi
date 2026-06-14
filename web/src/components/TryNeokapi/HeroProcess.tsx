import React, { useEffect, useState } from "react";
import clsx from "clsx";
import { FormatPreview } from "@neokapi/ui-primitives/preview";
import { ArrowRight, ChevronLeft, ChevronRight } from "lucide-react";
import { FRAMES, HERO_FILENAME, READ_FORMATS, STAGES } from "./heroStages";
import styles from "./styles.module.css";

// The hero: an auto-playing, six-stage "show" of kapi end to end —
// Read → Pre-process → Pseudo-translate → Leverage → Translate (ja) → Merge —
// rendered through the shared FormatPreview on BAKED data (heroStages.ts), so the
// page pulls ZERO wasm on load. The slide is an Acme pitch deck localized as a
// STACKED DECK: source cards on a peeking stack, then the pseudo and Japanese
// target cards slide in onto it. Within the cards, FormatPreview decodes terms in
// at Pre-process, keeps the confidential figures redacted throughout, and rolls
// translation-memory hits in at Leverage. Prev/next arrows step through stages, a
// counter + caption name the stage, and a CTA opens the live modal.
//
// prefers-reduced-motion: no auto-advance, no slide-in, no roll; step with arrows.

interface HeroProcessProps {
  /** Open the full modal. */
  onOpen: () => void;
}

// Per-stage dwell time (ms) before auto-advancing.
const DWELL = [2400, 3600, 3200, 3400, 3400, 2800];

// Stagger between line reveals on roll stages (ms per line index).
const LINE_STAGGER = 260;

// How many target cards have slid in by stage `i` — bumps only at the slide-in
// stages (pseudo, translate). Used as the card's key so it re-mounts (and the
// slide-in animation re-fires) exactly when a new target deck arrives, while the
// preview persists across the other stages so its line rolls have a "from" value.
function deckKey(i: number): number {
  return STAGES.slice(0, i + 1).filter((s) => FRAMES[s.key].slideIn).length;
}

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
  const [paused, setPaused] = useState(false);
  const stage = STAGES[i];
  const frame = FRAMES[stage.key];
  const n = STAGES.length;

  const go = (delta: number) => setI((p) => (p + delta + n) % n);

  // Auto-advance, looping; paused on hover/focus and under reduced motion.
  useEffect(() => {
    if (reduced || paused) return;
    const t = setTimeout(() => setI((p) => (p + 1) % n), DWELL[i]);
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

          {/* Stacked deck: two peeking back cards, then the active card. The
              deck tints by locale (en source → qps → ja target). */}
          <div className={styles.deck} data-locale={frame.locale}>
            <span className={styles.deckBackFar} aria-hidden="true" />
            <span className={styles.deckBackNear} aria-hidden="true" />
            <div
              key={deckKey(i)}
              className={clsx(
                styles.heroStage,
                stage.key === "read" && styles.heroStageRead,
                !reduced && frame.slideIn && styles.cardSlideIn,
              )}
            >
              <span className={styles.deckLabel} aria-hidden="true">
                <span className={styles.deckLabelLocale}>{frame.locale}</span>/{HERO_FILENAME}
              </span>
              <span className={styles.srOnly} aria-live="polite">
                {stage.label}: {stage.caption}
              </span>
              {frame.badge && (
                <span className={styles.heroBadge} key={`${stage.key}-badge`}>
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
              {stage.key === "read" && (
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
