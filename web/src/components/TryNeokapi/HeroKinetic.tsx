import React, { useEffect, useRef, useState } from "react";
import { AnimatePresence, MotionConfig, motion, useReducedMotion } from "motion/react";
import { ArrowRight, Check } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import styles from "./HeroKinetic.module.css";

// The landing centerpiece: a two-column "kinetic type + live asset" hero driven
// by Motion. A scene director advances through two looping phases on a fixed
// stage (~14s cycle):
//
//   LEFT  — a KINETIC VERB STACK. The verbs stack vertically; the ACTIVE verb is
//           large, bold, and dark; verbs already passed RECEDE (smaller, light
//           gray). The final verb SHIP resolves LARGE and GREEN with a ✓.
//   RIGHT — a LIVE ASSET (a document card styled as a brand guide) that visibly
//           transforms as each verb fires: rows populate (Shape), a body block
//           writes in (Write), green ✓ marks + a "brand ✓" chip appear (Check),
//           and a green "on brand" seal stamps on (Ship). It grounds the abstract
//           verbs in a real artifact — the thing being shaped, checked, shipped.
//
// PHASE 1 "The Content Loop" (one language): Shape → Write → Check → Ship.
// PHASE 2 "Going Multilingual" (every language): Read · Prep · Recycle ·
//   Translate · Check recede, then Ship resolves as stacked language forms
//   出荷 / Versand / Envío; the asset gains a de·ja·fr switcher, one row is
//   translated, and an "every language ✓" seal stamps.
//
// Zero wasm on load: this is pure JS animation; the engine boots only when the
// reader opens the modal (onOpen). SSR-safe: index.tsx guards this behind
// BrowserOnly with a static fallback frame. Under prefers-reduced-motion the hero
// renders a single static finished frame (Ship green ✓ + a finished brand guide,
// both "on brand" and "every language"), with no timers. The loop pauses on
// hover/focus.

interface HeroKineticProps {
  /** Open the live (wasm) modal. */
  onOpen: () => void;
}

const SPRING = { type: "spring" as const, stiffness: 320, damping: 26, mass: 0.9 };
const SOFT_SPRING = { type: "spring" as const, stiffness: 210, damping: 24 };

// ── The scene director ───────────────────────────────────────────────────────
// The hero is a strip of timed beats. Each beat names a `phase` (which act is on
// stage) and a `step` (how far into that act's choreography we are). A single
// timeout chain walks the strip; on the final beat it wraps to 0. Pausing simply
// stops scheduling the next beat, so the current frame holds.

type Phase = "content" | "multi";

interface Beat {
  phase: Phase;
  /** Progress index within the phase — the columns read it to advance. */
  step: number;
  /** Milliseconds this beat holds before the director advances. */
  hold: number;
}

// PHASE 1 (content) steps: 0 Shape, 1 Write, 2 Check, 3 Ship (+ hold).
// PHASE 2 (multi)   steps: 0 Read, 1 Prep, 2 Recycle, 3 Translate, 4 Check,
//                          5 Ship (+ hold).
const STRIP: Beat[] = [
  { phase: "content", step: 0, hold: 1700 },
  { phase: "content", step: 1, hold: 1800 },
  { phase: "content", step: 2, hold: 1700 },
  { phase: "content", step: 3, hold: 2400 },
  { phase: "multi", step: 0, hold: 1400 },
  { phase: "multi", step: 1, hold: 1400 },
  { phase: "multi", step: 2, hold: 1400 },
  { phase: "multi", step: 3, hold: 1500 },
  { phase: "multi", step: 4, hold: 1500 },
  { phase: "multi", step: 5, hold: 2600 },
];

// The verb stack for each phase. `climax` marks the SHIP verb (the one that
// resolves large + green). For phase 2 the SHIP verb renders as stacked language
// forms instead of a single word.
const CONTENT_VERBS = [
  t("Shape", "content loop verb"),
  t("Write", "content loop verb"),
  t("Check", "content loop verb"),
  t("Ship", "content loop verb"),
];
const MULTI_VERBS = [
  t("Read", "multilingual loop verb"),
  t("Prep", "multilingual loop verb"),
  t("Recycle", "multilingual loop verb"),
  t("Translate", "multilingual loop verb"),
  t("Check", "multilingual loop verb"),
  t("Ship", "multilingual loop verb"),
];
const SHIP_LANGS = ["出荷", "Versand", "Envío"];

// The brand-guide asset's definition rows (Shape populates these).
const GUIDE_ROWS = [
  // The row KEYS name kapi's own concepts, so they carry the context a
  // translator needs to keep the vocabulary the docs use.
  { k: t("Voice", "the voice profile, a kapi concept"), v: t("confident, plain") },
  { k: t("Terms", "the terms store, a kapi concept"), v: t("Acme (never ACME)") },
  { k: t("Tone", "a brand-guide row label"), v: t("concise") },
];
// The body content that writes in under Write.
const BODY_LINES = [
  t("Acme keeps promises simple.", "sample brand-guide body"),
  t("Say what it does, plainly.", "sample brand-guide body"),
];
// Cycled asset-type chip label — a subtle nod to breadth (brand guide · deck ·
// docs) while the concrete example stays a brand guide.
const ASSET_TYPES = ["brand guide", "deck", "docs"];
// The language switcher tabs shown in phase 2, and the translated row.
const LANG_TABS = ["de", "ja", "fr"];
const LOCALIZED_VOICE = "selbstbewusst, klar"; // de of "confident, plain"

// ── The kinetic verb stack (LEFT column) ─────────────────────────────────────

function KineticStack({
  verbs,
  active,
  climaxLangs,
  climaxLabel,
}: {
  verbs: string[];
  /** Index of the currently active verb (verbs before it have receded). */
  active: number;
  /** When the SHIP verb is active in phase 2, render these stacked forms. */
  climaxLangs?: string[];
  /** The small caption under the resolved SHIP verb. */
  climaxLabel: string;
}): React.ReactElement {
  const shipIndex = verbs.length - 1;
  const shipActive = active >= shipIndex;

  return (
    <div className={styles.stack} aria-hidden="true">
      {verbs.map((verb, i) => {
        const isShip = i === shipIndex;
        const isActive = i === active;
        const passed = i < active;

        // The receded (passed) verbs shrink to light gray; the active verb is
        // large + dark; upcoming verbs are dim placeholders. SHIP, once active,
        // resolves large + green.
        const state =
          isShip && shipActive ? "ship" : isActive ? "active" : passed ? "passed" : "upcoming";

        if (isShip && shipActive && climaxLangs) {
          // Phase 2: SHIP resolves as stacked language forms.
          return (
            <motion.div key={verb} className={styles.shipLangGroup} layout transition={SOFT_SPRING}>
              {climaxLangs.map((lang, k) => (
                <motion.span
                  key={lang}
                  className={styles.shipLang}
                  initial={{ opacity: 0, y: 14, scale: 0.8 }}
                  animate={{ opacity: 1, y: 0, scale: 1 }}
                  transition={{ ...SPRING, delay: k * 0.12 }}
                >
                  {lang}
                </motion.span>
              ))}
              <motion.span
                className={styles.climaxLabel}
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                transition={{ delay: 0.4, duration: 0.4 }}
              >
                <Check size={13} strokeWidth={3} /> {climaxLabel}
              </motion.span>
            </motion.div>
          );
        }

        return (
          <motion.div
            key={verb}
            className={styles.verbRow}
            data-state={state}
            layout
            transition={SOFT_SPRING}
          >
            <motion.span className={styles.verb} data-state={state} layout transition={SPRING}>
              {verb}
              <AnimatePresence>
                {isShip && shipActive && (
                  <motion.span
                    className={styles.verbCheck}
                    initial={{ scale: 0, opacity: 0 }}
                    animate={{ scale: 1, opacity: 1 }}
                    exit={{ scale: 0, opacity: 0 }}
                    transition={SPRING}
                  >
                    <Check size={26} strokeWidth={3} />
                  </motion.span>
                )}
              </AnimatePresence>
            </motion.span>
            <AnimatePresence>
              {isShip && shipActive && !climaxLangs && (
                <motion.span
                  className={styles.climaxLabel}
                  initial={{ opacity: 0, y: 4 }}
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: 0 }}
                  transition={{ delay: 0.15, duration: 0.4 }}
                >
                  {climaxLabel}
                </motion.span>
              )}
            </AnimatePresence>
          </motion.div>
        );
      })}
    </div>
  );
}

// ── The live brand-guide asset (RIGHT column) ────────────────────────────────

function AssetCard({
  // The gates below already encode which phase is on, so the phase itself is
  // destructured only to keep it out of the rest of the props.
  phase: _phase,
  typeLabel,
  // phase 1 gates
  rowsIn,
  bodyIn,
  checked,
  sealed,
  // phase 2 gates
  multi,
  langActive,
  localized,
  multiSealed,
}: {
  phase: Phase;
  typeLabel: string;
  rowsIn: boolean;
  bodyIn: boolean;
  checked: boolean;
  sealed: boolean;
  multi: boolean;
  langActive: string;
  localized: boolean;
  multiSealed: boolean;
}): React.ReactElement {
  return (
    <div className={styles.asset} aria-hidden="true">
      <div className={styles.assetHead}>
        <span className={styles.assetTitle}>Acme — Brand guide</span>
        <AnimatePresence mode="wait">
          <motion.span
            key={typeLabel}
            className={styles.assetType}
            initial={{ opacity: 0, y: -4 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: 4 }}
            transition={{ duration: 0.3 }}
          >
            {typeLabel}
          </motion.span>
        </AnimatePresence>
      </div>

      {/* Phase 2: the language switcher tabs appear above the content. */}
      <AnimatePresence>
        {multi && (
          <motion.div
            className={styles.langTabs}
            initial={{ opacity: 0, height: 0 }}
            animate={{ opacity: 1, height: "auto" }}
            exit={{ opacity: 0, height: 0 }}
            transition={SOFT_SPRING}
          >
            {LANG_TABS.map((t) => (
              <span
                key={t}
                className={styles.langTab}
                data-active={t === langActive ? "true" : "false"}
              >
                {t}
              </span>
            ))}
          </motion.div>
        )}
      </AnimatePresence>

      {/* Definition rows (Shape populates them). */}
      <div className={styles.rows}>
        {GUIDE_ROWS.map((row, i) => {
          const localizeThis = localized && i === 0;
          return (
            <motion.div
              key={row.k}
              className={styles.row}
              initial={{ opacity: 0, x: -10 }}
              animate={rowsIn ? { opacity: 1, x: 0 } : { opacity: 0, x: -10 }}
              transition={{ ...SPRING, delay: rowsIn ? i * 0.12 : 0 }}
            >
              <span className={styles.rowKey}>{row.k}</span>
              <AnimatePresence mode="wait">
                <motion.span
                  key={localizeThis ? "loc" : "src"}
                  className={styles.rowVal}
                  data-localized={localizeThis ? "true" : "false"}
                  initial={{ opacity: 0, y: 6 }}
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: 0, y: -6 }}
                  transition={{ duration: 0.35 }}
                >
                  {localizeThis ? LOCALIZED_VOICE : row.v}
                </motion.span>
              </AnimatePresence>
              {/* Check ✓ beside each row. */}
              <AnimatePresence>
                {checked && (
                  <motion.span
                    className={styles.rowCheck}
                    initial={{ scale: 0, opacity: 0 }}
                    animate={{ scale: 1, opacity: 1 }}
                    exit={{ scale: 0, opacity: 0 }}
                    transition={{ ...SPRING, delay: i * 0.08 }}
                  >
                    <Check size={12} strokeWidth={3} />
                  </motion.span>
                )}
              </AnimatePresence>
            </motion.div>
          );
        })}
      </div>

      {/* Body content block (Write types it in). */}
      <div className={styles.body}>
        <AnimatePresence>
          {bodyIn &&
            BODY_LINES.map((line, i) => (
              <motion.span
                key={line}
                className={styles.bodyLine}
                initial={{ opacity: 0, y: 6 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0 }}
                transition={{ ...SPRING, delay: i * 0.18 }}
              >
                {line}
              </motion.span>
            ))}
        </AnimatePresence>
      </div>

      {/* The "brand ✓" chip (Check). */}
      <div className={styles.chipRow}>
        <AnimatePresence>
          {checked && (
            <motion.span
              className={styles.brandChip}
              initial={{ opacity: 0, scale: 0.8 }}
              animate={{ opacity: 1, scale: 1 }}
              exit={{ opacity: 0, scale: 0.8 }}
              transition={SPRING}
            >
              <Check size={12} strokeWidth={3} /> lint · terms · review
            </motion.span>
          )}
        </AnimatePresence>
      </div>

      {/* The "on brand" / "every language" green seal (Ship). */}
      <AnimatePresence>
        {(sealed || multiSealed) && (
          <motion.div
            className={styles.seal}
            initial={{ opacity: 0, scale: 0.5, rotate: -12 }}
            animate={{ opacity: 1, scale: 1, rotate: -8 }}
            exit={{ opacity: 0, scale: 0.5 }}
            transition={SPRING}
          >
            <Check size={14} strokeWidth={3} />
            <span>{multiSealed ? "every language" : "on brand"}</span>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}

// ── The static (reduced-motion / SSR fallback) finished frame ────────────────

function StaticFrame(): React.ReactElement {
  return (
    <div className={styles.stage} aria-hidden="true">
      <div className={styles.stackCol}>
        <KineticStack
          verbs={CONTENT_VERBS}
          active={CONTENT_VERBS.length - 1}
          climaxLabel={t("on brand", "the seal stamped when content passes")}
        />
      </div>
      <div className={styles.assetCol}>
        <AssetCard
          phase="multi"
          typeLabel="brand guide"
          rowsIn
          bodyIn
          checked
          sealed={false}
          multi
          langActive="de"
          localized
          multiSealed
        />
      </div>
    </div>
  );
}

// The SSR / first-paint fallback: a non-interactive static finished frame. It is
// still the full-card button so the CTA is meaningful. index.tsx renders this on
// the server (and until the client mounts) so Motion never enters the SSR bundle.
export function HeroKineticFallback({ onOpen }: HeroKineticProps): React.ReactElement {
  return (
    <button
      type="button"
      className={styles.card}
      onClick={onOpen}
      aria-label="Open the interactive Kapi showcase"
    >
      <StaticFrame />
      <div className={styles.ctaRow}>
        <span className={styles.cta}>
          Try Kapi in your browser <ArrowRight size={16} aria-hidden="true" />
        </span>
      </div>
    </button>
  );
}

// ── The hero ─────────────────────────────────────────────────────────────────

export default function HeroKinetic({ onOpen }: HeroKineticProps): React.ReactElement {
  const reduced = useReducedMotion();
  const [cursor, setCursor] = useState(0);
  const [cycle, setCycle] = useState(0);
  const [paused, setPaused] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Dev/verification hook: `?heroBeat=<n>` pins the director to beat n (holds it,
  // no timers) so a headless screenshot can reliably capture a specific beat.
  const pinned = useRef<number | null>(null);
  useEffect(() => {
    const p = new URLSearchParams(window.location.search).get("heroBeat");
    if (p !== null) {
      const n = Number(p);
      if (Number.isInteger(n) && n >= 0 && n < STRIP.length) {
        pinned.current = n;
        setCursor(n);
      }
    }
  }, []);

  // The director: schedule the next beat after the current beat's hold. Pausing
  // (hover/focus) or reduced motion stops scheduling, holding the current frame.
  // On wrap, advance the cycle counter (used to rotate the asset-type chip).
  useEffect(() => {
    if (reduced || paused || pinned.current !== null) return;
    const beat = STRIP[cursor];
    timer.current = setTimeout(() => {
      setCursor((c) => {
        const next = (c + 1) % STRIP.length;
        if (next === 0) setCycle((y) => y + 1);
        return next;
      });
    }, beat.hold);
    return () => {
      if (timer.current) clearTimeout(timer.current);
    };
  }, [cursor, paused, reduced]);

  if (reduced) {
    return (
      <button
        type="button"
        className={styles.card}
        onClick={onOpen}
        aria-label="Open the interactive Kapi showcase"
      >
        <span className={styles.srOnly}>
          A brand guide, shaped and checked: its voice, terms, and tone are defined, body content is
          written, each rule passes review, and it ships on brand — then goes multilingual (de, ja,
          fr) and ships in every language.
        </span>
        <StaticFrame />
        <div className={styles.ctaRow}>
          <span className={styles.cta}>
            Try Kapi in your browser <ArrowRight size={16} aria-hidden="true" />
          </span>
        </div>
      </button>
    );
  }

  const beat = STRIP[cursor];
  const typeLabel = ASSET_TYPES[cycle % ASSET_TYPES.length];

  // Phase 1 (content) gates.
  const c = beat.phase === "content" ? beat.step : -1;
  const contentRowsIn = beat.phase === "content" && c >= 0;
  const contentBodyIn = beat.phase === "content" && c >= 1;
  const contentChecked = beat.phase === "content" && c >= 2;
  const contentSealed = beat.phase === "content" && c >= 3;

  // Phase 2 (multi) gates.
  const m = beat.phase === "multi" ? beat.step : -1;
  const multiRowsIn = beat.phase === "multi"; // rows carry over, populated
  const multiChecked = beat.phase === "multi" && m >= 4;
  const langActive = LANG_TABS[Math.min(m, 2)] ?? "de";
  const localized = beat.phase === "multi" && m >= 3;
  const multiSealed = beat.phase === "multi" && m >= 5;

  // Which verb is active in the current phase.
  const activeVerb = beat.phase === "content" ? beat.step : beat.step;
  const kicker = beat.phase === "content" ? "The Content Loop" : "Going Multilingual";

  return (
    <button
      type="button"
      className={styles.card}
      onClick={onOpen}
      onMouseEnter={() => setPaused(true)}
      onMouseLeave={() => setPaused(false)}
      onFocus={() => setPaused(true)}
      onBlur={() => setPaused(false)}
      aria-label="Open the interactive Kapi showcase — verbs act on a live brand guide that ships on brand, in every language"
    >
      <span className={styles.srOnly}>
        A self-playing loop. First, one language: Shape, Write, Check, Ship. A brand guide's voice,
        terms, and tone are defined, body content is written, each rule passes review, and it ships
        on brand. Then going multilingual: Read, Prep, Recycle, Translate, Check — the guide gains a
        de, ja, fr switcher, a rule localizes, and Ship resolves in every language. The loop
        repeats.
      </span>

      <MotionConfig reducedMotion="user">
        <div className={styles.stage} aria-hidden="true">
          {/* LEFT — the kinetic verb stack. */}
          <div className={styles.stackCol}>
            <span className={styles.kicker}>{kicker}</span>
            <AnimatePresence mode="wait">
              {beat.phase === "content" ? (
                <motion.div
                  key="content-stack"
                  initial={{ opacity: 0 }}
                  animate={{ opacity: 1 }}
                  exit={{ opacity: 0 }}
                  transition={{ duration: 0.3 }}
                >
                  <KineticStack
                    verbs={CONTENT_VERBS}
                    active={activeVerb}
                    climaxLabel={t("on brand", "the seal stamped when content passes")}
                  />
                </motion.div>
              ) : (
                <motion.div
                  key="multi-stack"
                  initial={{ opacity: 0 }}
                  animate={{ opacity: 1 }}
                  exit={{ opacity: 0 }}
                  transition={{ duration: 0.3 }}
                >
                  <KineticStack
                    verbs={MULTI_VERBS}
                    active={activeVerb}
                    climaxLangs={SHIP_LANGS}
                    climaxLabel={t("every language", "the seal stamped when every locale passes")}
                  />
                </motion.div>
              )}
            </AnimatePresence>
          </div>

          {/* RIGHT — the live brand-guide asset. */}
          <div className={styles.assetCol}>
            <AssetCard
              phase={beat.phase}
              typeLabel={typeLabel}
              rowsIn={contentRowsIn || multiRowsIn}
              bodyIn={contentBodyIn || beat.phase === "multi"}
              checked={contentChecked || multiChecked}
              sealed={contentSealed}
              multi={beat.phase === "multi"}
              langActive={langActive}
              localized={localized}
              multiSealed={multiSealed}
            />
          </div>
        </div>
      </MotionConfig>

      <div className={styles.ctaRow}>
        <span className={styles.cta}>
          Try Kapi in your browser <ArrowRight size={16} aria-hidden="true" />
        </span>
      </div>
    </button>
  );
}
