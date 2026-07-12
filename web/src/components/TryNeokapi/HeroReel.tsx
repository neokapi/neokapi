import React, { useEffect, useRef, useState } from "react";
import { AnimatePresence, MotionConfig, motion, useReducedMotion } from "motion/react";
import { ArrowRight, Check } from "lucide-react";
import styles from "./HeroReel.module.css";

// The landing centerpiece: a cinematic, self-playing "content loop" reel driven
// by Motion. A scene director advances through four phases on a fixed stage and
// loops forever (~14s cycle):
//
//   1. Title card "The Content Loop" pulses in / holds / pulses out.
//   2. The single-language content loop animates: Shape → Write → Check nodes
//      spring in, the Write⇄Check refine iteration visibly bounces (it iterates),
//      then a token streams right and CONVERGES into a persistent green ship gate.
//   3. Title card "Going Multilingual" pulses in / holds / out.
//   4. The localization loop animates: Read → Prep → Recycle → Translate → Check
//      nodes spring in; format chips stream in at Read, memory segments snap in at
//      Recycle, language tokens shoot in at Translate; then multiple language chips
//      stream right and CONVERGE into the same gate. Crossfade back to phase 1.
//
// A persistent SHIP GATE is anchored at the right of the stage — dim by default,
// it lights solid green with a ✓ every time a loop converges into it. It is the
// recurring climax that ties both loops together.
//
// Zero wasm on load: this is pure JS animation; the engine boots only when the
// reader opens the modal (onOpen). SSR-safe: index.tsx guards this behind
// BrowserOnly with a static fallback frame. Under prefers-reduced-motion the reel
// renders a single static frame (both loops as a diagram, both gates green) with
// no timers. The loop pauses on hover/focus.

interface HeroReelProps {
  /** Open the live (wasm) modal. */
  onOpen: () => void;
}

// A node is a labeled pill in a loop.
interface Node {
  verb: string;
  sub: string;
}

const CONTENT_NODES: Node[] = [
  { verb: "Shape", sub: "terms · brand · tone" },
  { verb: "Write", sub: "you or your AI" },
  { verb: "Check", sub: "lint · terms · review" },
];

const MULTI_NODES: Node[] = [
  { verb: "Read", sub: "any format" },
  { verb: "Prep", sub: "protect · redact" },
  { verb: "Recycle", sub: "reuse from memory" },
  { verb: "Translate", sub: "your AI" },
  { verb: "Check", sub: "red-line · green-line" },
];

const READ_FORMATS = ["PPTX", "JSON", "DOCX"];
const SHIP_LANGS = ["de", "ja", "fr"];

// ── The scene director ───────────────────────────────────────────────────────
// The reel is a strip of timed beats. Each beat sets a `phase` (which act is on
// stage) and a `step` (how far into that act's choreography we are). A single
// timeout chain walks the strip; on the final beat it wraps to 0. Pausing simply
// stops scheduling the next beat, so the current frame holds.

type Phase = "title-1" | "content" | "title-2" | "multi";

interface Beat {
  phase: Phase;
  /** Progress index within the phase — components read it to reveal nodes/tokens. */
  step: number;
  /** Milliseconds this beat holds before the director advances. */
  hold: number;
}

// The strip. Steps within a phase gate the staggered reveals + the convergence.
// content: 0 nodes-in, 1 refine iterates, 2 converge/gate lit.
// multi: 0 nodes-in (with format/memory/lang inflows), 1 converge/gate lit.
const STRIP: Beat[] = [
  { phase: "title-1", step: 0, hold: 2200 },
  { phase: "content", step: 0, hold: 1500 },
  { phase: "content", step: 1, hold: 1900 },
  { phase: "content", step: 2, hold: 1600 },
  { phase: "title-2", step: 0, hold: 2200 },
  { phase: "multi", step: 0, hold: 3200 },
  { phase: "multi", step: 1, hold: 1800 },
];

const SPRING = { type: "spring" as const, stiffness: 340, damping: 26, mass: 0.9 };
const SOFT_SPRING = { type: "spring" as const, stiffness: 220, damping: 24 };

// ── Title card ───────────────────────────────────────────────────────────────

function TitleCard({ title, sub }: { title: string; sub: string }): React.ReactElement {
  return (
    <motion.div
      className={styles.titleCard}
      initial={{ opacity: 0, scale: 0.92, filter: "blur(6px)" }}
      animate={{ opacity: 1, scale: 1, filter: "blur(0px)" }}
      exit={{ opacity: 0, scale: 1.04, filter: "blur(5px)" }}
      transition={SOFT_SPRING}
      aria-hidden="true"
    >
      <motion.span
        className={styles.titleText}
        initial={{ y: 8 }}
        animate={{ y: 0 }}
        transition={SPRING}
      >
        {title}
      </motion.span>
      <motion.span
        className={styles.titleSub}
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        transition={{ delay: 0.25, duration: 0.4 }}
      >
        {sub}
      </motion.span>
    </motion.div>
  );
}

// ── A loop node (springing pill) ─────────────────────────────────────────────

function LoopNode({
  node,
  index,
  visible,
  flash,
}: {
  node: Node;
  index: number;
  visible: boolean;
  /** When true, the node flashes a ✓ (the Check node passing). */
  flash?: boolean;
}): React.ReactElement {
  return (
    <motion.div
      className={styles.node}
      initial={{ opacity: 0, scale: 0.6, y: 10 }}
      animate={visible ? { opacity: 1, scale: 1, y: 0 } : { opacity: 0, scale: 0.6, y: 10 }}
      transition={{ ...SPRING, delay: visible ? index * 0.16 : 0 }}
    >
      <span className={styles.nodeVerb}>
        {node.verb}
        <AnimatePresence>
          {flash && (
            <motion.span
              key="check"
              className={styles.nodeCheck}
              initial={{ scale: 0, opacity: 0 }}
              animate={{ scale: 1, opacity: 1 }}
              exit={{ scale: 0, opacity: 0 }}
              transition={SPRING}
            >
              <Check size={11} strokeWidth={3} />
            </motion.span>
          )}
        </AnimatePresence>
      </span>
      <span className={styles.nodeSub}>{node.sub}</span>
    </motion.div>
  );
}

// A short connector drawn between nodes.
function Connector({ visible, delay }: { visible: boolean; delay: number }): React.ReactElement {
  return (
    <motion.span
      className={styles.connector}
      aria-hidden="true"
      initial={{ scaleX: 0, opacity: 0 }}
      animate={visible ? { scaleX: 1, opacity: 1 } : { scaleX: 0, opacity: 0 }}
      transition={{ ...SOFT_SPRING, delay: visible ? delay : 0 }}
    />
  );
}

// ── The ship gate (persistent, right-anchored climax) ────────────────────────

function ShipGate({ lit, label }: { lit: boolean; label: string }): React.ReactElement {
  return (
    <div className={styles.gateWrap} aria-hidden="true">
      <motion.div
        className={styles.gate}
        animate={
          lit
            ? {
                borderColor: "var(--hr-green)",
                backgroundColor: "var(--hr-green-fill)",
                boxShadow: "0 0 0 6px var(--hr-green-glow)",
              }
            : {
                borderColor: "var(--hr-border)",
                backgroundColor: "transparent",
                boxShadow: "0 0 0 0px transparent",
              }
        }
        transition={SOFT_SPRING}
      >
        <AnimatePresence mode="wait">
          {lit ? (
            <motion.span
              key="check"
              className={styles.gateCheck}
              initial={{ scale: 0, opacity: 0 }}
              animate={{ scale: 1, opacity: 1 }}
              exit={{ scale: 0, opacity: 0 }}
              transition={SPRING}
            >
              <Check size={22} strokeWidth={3} />
            </motion.span>
          ) : (
            <motion.span
              key="ship"
              className={styles.gateIdle}
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0 }}
            >
              Ship
            </motion.span>
          )}
        </AnimatePresence>
      </motion.div>
      <motion.span
        className={styles.gateLabel}
        animate={{ opacity: lit ? 1 : 0.45, color: lit ? "var(--hr-green)" : "var(--hr-muted)" }}
        transition={{ duration: 0.35 }}
      >
        {lit ? label : "ship gate"}
      </motion.span>
    </div>
  );
}

// ── A flying token (format chip, memory segment, or language) ────────────────

function flyVariants(from: "left" | "right", drift: number) {
  return {
    initial: { opacity: 0, x: from === "left" ? -70 : 70, y: drift, scale: 0.7 },
    animate: { opacity: 1, x: 0, y: 0, scale: 1 },
    exit: { opacity: 0, scale: 0.7 },
  };
}

// ── The content loop act ─────────────────────────────────────────────────────

function ContentAct({ step }: { step: number }): React.ReactElement {
  // step 0: nodes spring in. step 1: refine token bounces Write⇄Check. step 2:
  // token streams to gate, Check flashes ✓, gate lights.
  const nodesIn = step >= 0;
  const refining = step === 1;
  const converged = step >= 2;

  return (
    <div className={styles.act}>
      <div className={styles.loopRow}>
        <LoopNode node={CONTENT_NODES[0]} index={0} visible={nodesIn} />
        <Connector visible={nodesIn} delay={0.2} />
        <LoopNode node={CONTENT_NODES[1]} index={1} visible={nodesIn} />
        <Connector visible={nodesIn} delay={0.36} />
        <div className={styles.checkCol}>
          <LoopNode
            node={CONTENT_NODES[2]}
            index={2}
            visible={nodesIn}
            flash={refining || converged}
          />
          {/* The Write⇄Check refine iteration: a token bounces back and forth. */}
          <AnimatePresence>
            {refining && (
              <motion.span
                key="refine"
                className={styles.refineToken}
                initial={{ opacity: 0 }}
                animate={{
                  opacity: 1,
                  x: [0, -78, 0, -78, 0],
                }}
                exit={{ opacity: 0 }}
                transition={{ duration: 1.7, times: [0, 0.25, 0.5, 0.75, 1], ease: "easeInOut" }}
                aria-hidden="true"
              />
            )}
          </AnimatePresence>
        </div>
      </div>

      {/* The convergence stream: a token flows from Check rightward into the gate. */}
      <div className={styles.streamLane}>
        <AnimatePresence>
          {converged && (
            <motion.span
              key="stream"
              className={styles.streamToken}
              initial={{ opacity: 0, x: 0, scale: 0.8 }}
              animate={{ opacity: [0, 1, 1, 0], x: [0, 60, 130, 190], scale: 1 }}
              transition={{ duration: 0.9, ease: "easeIn" }}
              aria-hidden="true"
            />
          )}
        </AnimatePresence>
      </div>
    </div>
  );
}

// ── The multilingual loop act ────────────────────────────────────────────────

function MultiAct({ step }: { step: number }): React.ReactElement {
  const nodesIn = step >= 0;
  const converged = step >= 1;

  return (
    <div className={styles.act}>
      <div className={styles.multiRow}>
        {MULTI_NODES.map((node, i) => (
          <React.Fragment key={node.verb}>
            {i > 0 && <Connector visible={nodesIn} delay={0.16 + i * 0.14} />}
            <div className={styles.multiCol}>
              <LoopNode node={node} index={i} visible={nodesIn} flash={i === 4 && converged} />

              {/* Read: format chips stream in from the left. */}
              {i === 0 && (
                <div className={styles.inflow}>
                  <AnimatePresence>
                    {nodesIn &&
                      READ_FORMATS.map((f, k) => (
                        <motion.span
                          key={f}
                          className={styles.chip}
                          {...flyVariants("left", -6 + k * 6)}
                          transition={{ ...SPRING, delay: 0.5 + k * 0.14 }}
                          aria-hidden="true"
                        >
                          {f}
                        </motion.span>
                      ))}
                  </AnimatePresence>
                </div>
              )}

              {/* Recycle: memory segments snap in. */}
              {i === 2 && (
                <div className={styles.inflow}>
                  <AnimatePresence>
                    {nodesIn &&
                      [0, 1].map((k) => (
                        <motion.span
                          key={k}
                          className={styles.memToken}
                          initial={{ opacity: 0, x: -30, scale: 0.6 }}
                          animate={{ opacity: 1, x: 0, scale: 1 }}
                          transition={{ ...SPRING, delay: 0.9 + k * 0.18 }}
                          aria-hidden="true"
                        />
                      ))}
                  </AnimatePresence>
                </div>
              )}

              {/* Translate: language tokens shoot in. */}
              {i === 3 && (
                <div className={styles.inflow}>
                  <AnimatePresence>
                    {nodesIn &&
                      SHIP_LANGS.map((l, k) => (
                        <motion.span
                          key={l}
                          className={styles.langChip}
                          initial={{ opacity: 0, y: 16, scale: 0.6 }}
                          animate={{ opacity: 1, y: 0, scale: 1 }}
                          transition={{ ...SPRING, delay: 1.2 + k * 0.13 }}
                          aria-hidden="true"
                        >
                          {l}
                        </motion.span>
                      ))}
                  </AnimatePresence>
                </div>
              )}
            </div>
          </React.Fragment>
        ))}
      </div>

      {/* Convergence: multiple language chips stream rightward into the gate. */}
      <div className={styles.streamLane}>
        <AnimatePresence>
          {converged &&
            SHIP_LANGS.map((l, k) => (
              <motion.span
                key={l}
                className={styles.langStream}
                initial={{ opacity: 0, x: 0, scale: 0.8 }}
                animate={{ opacity: [0, 1, 1, 0], x: [0, 70, 150, 220], scale: 1 }}
                transition={{ duration: 1, ease: "easeIn", delay: k * 0.12 }}
                aria-hidden="true"
              >
                {l}
              </motion.span>
            ))}
        </AnimatePresence>
      </div>
    </div>
  );
}

// ── The static (reduced-motion) frame ────────────────────────────────────────

function StaticFrame(): React.ReactElement {
  return (
    <div className={styles.staticFrame} aria-hidden="true">
      <div className={styles.staticLoop}>
        <span className={styles.staticTitle}>The Content Loop</span>
        <div className={styles.staticRow}>
          {CONTENT_NODES.map((n) => (
            <span key={n.verb} className={styles.staticPill}>
              {n.verb}
            </span>
          ))}
          <span className={styles.staticArrow}>→</span>
          <span className={styles.staticGate}>
            <Check size={14} strokeWidth={3} /> on brand
          </span>
        </div>
      </div>
      <div className={styles.staticLoop}>
        <span className={styles.staticTitle}>Going Multilingual</span>
        <div className={styles.staticRow}>
          {MULTI_NODES.map((n) => (
            <span key={n.verb} className={styles.staticPill}>
              {n.verb}
            </span>
          ))}
          <span className={styles.staticArrow}>→</span>
          <span className={styles.staticGate}>
            <Check size={14} strokeWidth={3} /> every language
          </span>
        </div>
      </div>
    </div>
  );
}

// The SSR / first-paint fallback: a non-interactive static card showing the same
// diagram as the reduced-motion frame. index.tsx renders this on the server (and
// until the client mounts) so Motion never enters the SSR bundle and there is no
// hydration flash. It is still the full-card button so the CTA is meaningful.
export function HeroReelFallback({ onOpen }: HeroReelProps): React.ReactElement {
  return (
    <button type="button" className={styles.card} onClick={onOpen} aria-label="Open the interactive Kapi showcase">
      <div className={styles.stage}>
        <div className={styles.scene}>
          <StaticFrame />
        </div>
      </div>
      <div className={styles.ctaRow}>
        <span className={styles.cta}>
          Try Kapi in your browser <ArrowRight size={16} aria-hidden="true" />
        </span>
      </div>
    </button>
  );
}

// ── The reel ─────────────────────────────────────────────────────────────────

export default function HeroReel({ onOpen }: HeroReelProps): React.ReactElement {
  const reduced = useReducedMotion();
  const [cursor, setCursor] = useState(0);
  const [paused, setPaused] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  // The director: schedule the next beat after the current beat's hold. Pausing
  // (hover/focus) or reduced motion stops scheduling, holding the current frame.
  useEffect(() => {
    if (reduced || paused) return;
    const beat = STRIP[cursor];
    timer.current = setTimeout(() => {
      setCursor((c) => (c + 1) % STRIP.length);
    }, beat.hold);
    return () => {
      if (timer.current) clearTimeout(timer.current);
    };
  }, [cursor, paused, reduced]);

  const beat = STRIP[cursor];

  // The gate lights on the convergence beats of each act and stays lit through
  // the following title crossfade for a satisfying resolve.
  const contentConverged = beat.phase === "content" && beat.step >= 2;
  const multiConverged = beat.phase === "multi" && beat.step >= 1;
  const gateLit = reduced || contentConverged || multiConverged;
  const gateLabel = multiConverged ? "every language" : "on brand";

  return (
    <button
      type="button"
      className={styles.card}
      onClick={onOpen}
      onMouseEnter={() => setPaused(true)}
      onMouseLeave={() => setPaused(false)}
      onFocus={() => setPaused(true)}
      onBlur={() => setPaused(false)}
      aria-label="Open the interactive Kapi showcase — content flows around two loops that each converge into a ship gate"
    >
      <span className={styles.srOnly}>
        A self-playing reel of the content loop. First, the single-language content
        loop: Shape, Write, Check — the Write and Check steps iterate to refine the
        text, which then converges into a ship gate that turns green: on brand. Then
        going multilingual: Read any format, Prep, Recycle from memory, Translate,
        Check — every language converges into the same green ship gate. The loop
        repeats.
      </span>

      {reduced ? (
        <StaticFrame />
      ) : (
        <MotionConfig reducedMotion="user">
          <div className={styles.stage} aria-hidden="true">
            <ShipGate lit={gateLit} label={gateLabel} />

            <div className={styles.scene}>
              <AnimatePresence mode="wait">
                {beat.phase === "title-1" && (
                  <TitleCard key="t1" title="The Content Loop" sub="one language" />
                )}
                {beat.phase === "content" && (
                  <motion.div
                    key="content"
                    className={styles.actWrap}
                    initial={{ opacity: 0 }}
                    animate={{ opacity: 1 }}
                    exit={{ opacity: 0 }}
                    transition={{ duration: 0.4 }}
                  >
                    <ContentAct step={beat.step} />
                  </motion.div>
                )}
                {beat.phase === "title-2" && (
                  <TitleCard key="t2" title="Going Multilingual" sub="every language" />
                )}
                {beat.phase === "multi" && (
                  <motion.div
                    key="multi"
                    className={styles.actWrap}
                    initial={{ opacity: 0 }}
                    animate={{ opacity: 1 }}
                    exit={{ opacity: 0 }}
                    transition={{ duration: 0.4 }}
                  >
                    <MultiAct step={beat.step} />
                  </motion.div>
                )}
              </AnimatePresence>
            </div>
          </div>
        </MotionConfig>
      )}

      <div className={styles.ctaRow}>
        <span className={styles.cta}>
          Try Kapi in your browser <ArrowRight size={16} aria-hidden="true" />
        </span>
      </div>
    </button>
  );
}
