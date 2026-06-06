import React, { useEffect, useState } from "react";
import FormatPreview from "@neokapi/kapi-lab/FormatPreview";
import { Combine, Database, Languages, ScanText, ShieldCheck, Wand2 } from "lucide-react";
import { FRAMES, HERO_FILENAME, READ_FORMATS, STAGES, type StageKey } from "./heroStages";
import styles from "./styles.module.css";

// The hero: an auto-playing, six-stage "show" of kapi end to end —
// Read → Pre-process → Pseudo-translate → Leverage → Translate (ja) → Merge —
// rendered through the shared FormatPreview on BAKED data (heroProcess.ts), so
// the page pulls ZERO wasm on load. FormatPreview's typewriter/crossfade carries
// the source → pseudo → Japanese progression; a stepper tracks the stage and the
// whole card is one button that opens the live modal.
//
// prefers-reduced-motion: no auto-advance and no typewriter; the reader steps
// through the stages with the stepper instead.

interface HeroProcessProps {
  /** Open the full modal (the card is one big button). */
  onOpen: () => void;
}

const STAGE_ICON: Record<StageKey, React.ComponentType<{ size?: number; className?: string }>> = {
  read: ScanText,
  preprocess: ShieldCheck,
  pseudo: Wand2,
  leverage: Database,
  translate: Languages,
  merge: Combine,
};

// Per-stage dwell time (ms). The typewriter stages get a touch longer.
const DWELL: Record<StageKey, number> = {
  read: 2400,
  preprocess: 3400,
  pseudo: 2900,
  leverage: 2900,
  translate: 3400,
  merge: 3000,
};

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

  // Auto-advance through the stages, looping. Paused on hover/focus and under
  // reduced motion.
  useEffect(() => {
    if (reduced || paused) return;
    const t = setTimeout(() => setI((p) => (p + 1) % STAGES.length), DWELL[stage.key]);
    return () => clearTimeout(t);
  }, [i, reduced, paused, stage.key]);

  return (
    <div
      className={styles.heroCard}
      onMouseEnter={() => setPaused(true)}
      onMouseLeave={() => setPaused(false)}
    >
      {/* Stepper */}
      <div className={styles.heroStepper} role="tablist" aria-label="kapi pipeline stages">
        {STAGES.map((s, idx) => {
          const Icon = STAGE_ICON[s.key];
          const state = idx === i ? "active" : idx < i ? "done" : "todo";
          return (
            <button
              key={s.key}
              type="button"
              role="tab"
              aria-selected={idx === i}
              className={`${styles.heroStep} ${styles[`heroStep_${state}`]}`}
              onClick={() => setI(idx)}
            >
              <span className={styles.heroStepDot} aria-hidden="true">
                <Icon size={14} />
              </span>
              <span className={styles.heroStepLabel}>{s.short}</span>
            </button>
          );
        })}
      </div>

      {/* The stage — one big button into the live modal. */}
      <button
        type="button"
        className={styles.heroOpen}
        onClick={onOpen}
        aria-label="Open the interactive Try Neokapi showcase"
      >
        <div className={styles.heroChrome}>
          <span className={styles.heroFile}>{HERO_FILENAME}</span>
          <span className={styles.heroStageName}>
            {i + 1}/{STAGES.length} · {stage.label}
          </span>
        </div>

        <div className={styles.heroStage}>
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
            typewriter="char"
            reducedMotion={reduced}
            gridHeaders={false}
            className={styles.heroDoc}
          />
          {stage.key === "read" && (
            <div className={styles.heroFormats} aria-hidden="true">
              {READ_FORMATS.map((f) => (
                <span key={f} className={styles.heroFormatChip}>
                  {f}
                </span>
              ))}
              <span className={`${styles.heroFormatChip} ${styles.heroFormatMore}`}>50+</span>
            </div>
          )}
        </div>
      </button>

      {/* Caption */}
      <p className={styles.heroCaption} key={`${stage.key}-cap`}>
        <strong>{stage.label}.</strong> {stage.caption}
      </p>
    </div>
  );
}
