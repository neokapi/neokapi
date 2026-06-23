import React, { useEffect, useState } from "react";
import {
  animate,
  AnimatePresence,
  motion,
  MotionConfig,
  useMotionValue,
  useReducedMotion,
  useTransform,
} from "motion/react";
import { ArrowRight, Check } from "lucide-react";
import {
  BRAND,
  type DocLine,
  READ_FORMATS,
  type Role,
  type Scene,
  SCENES,
  SHIP_LANGS,
} from "./loopScenes";
import styles from "./heroLoop.module.css";

// The landing centerpiece: a Motion-driven "Content Loop". A themed SVG loop
// (the docs diagram-kit palette) carries a springing traveler through the six
// stages of going multilingual — Read → Prep → Recycle → Translate → Check →
// Ship → repeat — fed by authoring. Below it, one persistent slide morphs through
// every stage on a single card: terms get marked, confidential figures get
// redacted before they travel, recycled segments snap in from memory, the rest
// fills with AI, a reviewer green-lines it, and the file is written back
// byte-for-byte in every language. The active loop node and the card share the
// stage's role color, so the eye links "this stage" to "this change".
//
// Zero wasm on load: the content is baked (loopScenes.ts); the engine boots only
// when the reader opens the modal. SSR-safe (Motion renders a sensible first
// paint; `initial={false}` suppresses an entrance flash). Under
// prefers-reduced-motion the loop settles on its finished, every-language state
// with no timers and no traveler — a legible static end-state.

interface HeroLoopProps {
  /** Open the live (wasm) modal. */
  onOpen: () => void;
}

const N = SCENES.length;

// ── Loop geometry (a wide ellipse; node 0 at top, clockwise) ─────────────────
const VB_W = 384;
const VB_H = 158;
const CX = 192;
const CY = 73;
const RX = 150;
const RY = 48;
const A0 = -Math.PI / 2;
const STEP = (2 * Math.PI) / N;
const nodeAngle = (i: number): number => A0 + i * STEP;
const ex = (a: number): number => CX + RX * Math.cos(a);
const ey = (a: number): number => CY + RY * Math.sin(a);

// Per-stage dwell (ms): the heavier transforms (translate, ship) get more time.
const DWELL = [2400, 2600, 2600, 2900, 2600, 3000];

const roleVar = (role: Role): string => `var(--hl-${role})`;

// SVG label anchor + offset per node, so labels sit just outside the ellipse and
// never collide with the ring.
const LABELS: { anchor: "start" | "middle" | "end"; dx: number; dy: number }[] = [
  { anchor: "middle", dx: 0, dy: -13 }, // Read (top)
  { anchor: "start", dx: 13, dy: 1 }, // Prep (upper right)
  { anchor: "start", dx: 13, dy: 5 }, // Recycle (lower right)
  { anchor: "middle", dx: 0, dy: 19 }, // Translate (bottom)
  { anchor: "end", dx: -13, dy: 5 }, // Check (lower left)
  { anchor: "end", dx: -13, dy: 1 }, // Ship (upper left)
];

// ── The loop ─────────────────────────────────────────────────────────────────

function Loop({ idx, reduced }: { idx: number; reduced: boolean }): React.ReactElement {
  // A monotonically increasing angle so the traveler always moves forward —
  // including through the closing seam (Ship → Read) — rather than snapping back.
  const angle = useMotionValue(nodeAngle(idx));
  const tx = useTransform(angle, (a) => ex(a));
  const ty = useTransform(angle, (a) => ey(a));

  useEffect(() => {
    if (reduced) return;
    const controls = animate(angle, nodeAngle(idx), {
      type: "spring",
      stiffness: 80,
      damping: 17,
      mass: 0.9,
    });
    return () => controls.stop();
    // angle is a stable MotionValue.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [idx, reduced]);

  return (
    <svg
      className={styles.loopSvg}
      viewBox={`0 0 ${VB_W} ${VB_H}`}
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden="true"
    >
      <defs>
        <filter id="hl-glow" x="-60%" y="-60%" width="220%" height="220%">
          <feGaussianBlur stdDeviation="4.5" />
        </filter>
        <marker id="hl-arrow" markerWidth="7" markerHeight="7" refX="4.5" refY="3" orient="auto">
          <path d="M0,0 L6,3 L0,6 Z" className={styles.arrowHead} />
        </marker>
      </defs>

      {/* Faint full track + the per-segment "fill" that lights as the traveler
          passes (segment i, node i → node i+1, lights once stage i is behind us). */}
      <ellipse cx={CX} cy={CY} rx={RX} ry={RY} className={styles.track} />
      {SCENES.map((_, i) => {
        const a1 = nodeAngle(i);
        const a2 = nodeAngle(i + 1);
        const lit = !reduced && i < idx;
        return (
          <path
            key={`seg-${i}`}
            d={`M ${ex(a1)},${ey(a1)} A ${RX},${RY} 0 0 1 ${ex(a2)},${ey(a2)}`}
            className={lit || reduced ? styles.segLit : styles.seg}
            style={{ "--seg": roleVar(SCENES[i].role) } as React.CSSProperties}
            markerMid="url(#hl-arrow)"
          />
        );
      })}

      {/* Direction chevrons at the three "between" midpoints. */}
      {[0.5, 2.5, 4.5].map((m) => {
        const a = nodeAngle(m);
        const t = a + Math.PI / 2; // tangent
        return (
          <path
            key={`chev-${m}`}
            d="M -3,-2.4 L 3,0 L -3,2.4"
            className={styles.chevron}
            transform={`translate(${ex(a)},${ey(a)}) rotate(${(t * 180) / Math.PI})`}
          />
        );
      })}

      {/* "Authoring" feeder into Read — loop 1 (get the source right) feeds loop 2. */}
      <path
        d={`M ${CX - 30},6 C ${CX - 12},2 ${CX + 4},6 ${ex(A0)},${ey(A0) - 9}`}
        className={styles.feeder}
        markerEnd="url(#hl-arrow)"
      />
      <text x={CX - 32} y={6} textAnchor="end" className={styles.feederLabel}>
        authoring
      </text>

      {/* "repeat" tag on the closing seam (top-left → top). */}
      <text x={ex(A0 - STEP / 2) - 4} y={ey(A0 - STEP / 2) - 6} className={styles.repeatLabel}>
        ↻ repeat
      </text>

      {/* Nodes + labels. Each node keeps its own role color; the active node
          saturates, scales, and gets a focus ring. */}
      {SCENES.map((s, i) => {
        const a = nodeAngle(i);
        const active = i === idx;
        const passed = !reduced && i < idx;
        const lab = LABELS[i];
        return (
          <g key={s.key} style={{ "--n": roleVar(s.role) } as React.CSSProperties}>
            {active && <circle cx={ex(a)} cy={ey(a)} r={11} className={styles.nodeRing} />}
            <circle
              cx={ex(a)}
              cy={ey(a)}
              r={active ? 6 : 4.5}
              className={
                active ? styles.nodeActive : passed || reduced ? styles.nodePassed : styles.node
              }
            />
            <text
              x={ex(a) + lab.dx}
              y={ey(a) + lab.dy}
              textAnchor={lab.anchor}
              className={active ? styles.nodeLabelActive : styles.nodeLabel}
            >
              {s.stage}
            </text>
          </g>
        );
      })}

      {/* The traveler — a glowing pip riding the ring (hidden under reduced motion). */}
      {!reduced && (
        <g>
          <motion.circle
            cx={tx}
            cy={ty}
            r={9}
            className={styles.travelerHalo}
            filter="url(#hl-glow)"
          />
          <motion.circle cx={tx} cy={ty} r={4} className={styles.traveler} />
        </g>
      )}
    </svg>
  );
}

// ── The morphing slide ───────────────────────────────────────────────────────

/** Render a text line, drawing the protected brand term with an animated rule. */
function LineText({ line }: { line: Extract<DocLine, { kind: "text" }> }): React.ReactElement {
  const parts = line.term ? line.text.split(BRAND) : [line.text];
  return (
    <span className={styles.lineText} data-target={line.target ? "" : undefined}>
      {parts.map((part, i) => (
        <React.Fragment key={i}>
          {i > 0 && (
            <span className={styles.term}>
              {BRAND}
              <motion.span
                className={styles.termRule}
                initial={{ scaleX: 0 }}
                animate={{ scaleX: 1 }}
                transition={{ duration: 0.4, ease: "easeOut" }}
              />
            </span>
          )}
          {part}
        </React.Fragment>
      ))}
    </span>
  );
}

/** Render a figure line: the confidential span is masked by a redaction bar
 *  while redacted, and the real figure springs back in on Ship. */
function LineFigure({ line }: { line: Extract<DocLine, { kind: "figure" }> }): React.ReactElement {
  return (
    <span className={styles.lineText} data-target={line.target ? "" : undefined}>
      <AnimatePresence mode="wait" initial={false}>
        <motion.span
          key={line.pre}
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          transition={{ duration: 0.3 }}
        >
          {line.pre}
        </motion.span>
      </AnimatePresence>
      <AnimatePresence mode="wait" initial={false}>
        {line.redacted ? (
          <motion.span
            key="bar"
            className={styles.redact}
            style={{ width: `${Math.max(2.2, line.figure.length * 0.62)}em` }}
            initial={{ scaleX: 0, opacity: 0 }}
            animate={{ scaleX: 1, opacity: 1 }}
            exit={{ scaleX: 0, opacity: 0 }}
            transition={{ type: "spring", stiffness: 280, damping: 26 }}
            aria-label="redacted"
          />
        ) : (
          <motion.span
            key="fig"
            className={styles.figure}
            initial={{ opacity: 0, y: 3 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0 }}
            transition={{ type: "spring", stiffness: 300, damping: 24 }}
          >
            {line.figure}
          </motion.span>
        )}
      </AnimatePresence>
      <span>{line.post}</span>
    </span>
  );
}

function SlideLine({ line }: { line: DocLine }): React.ReactElement {
  return (
    <motion.li layout className={styles.line} data-role={line.id === "t" ? "title" : "bullet"}>
      <AnimatePresence initial={false}>
        {line.approved && (
          <motion.span
            key="ok"
            className={styles.approve}
            initial={{ scale: 0, opacity: 0 }}
            animate={{ scale: 1, opacity: 1 }}
            exit={{ scale: 0, opacity: 0 }}
            transition={{ type: "spring", stiffness: 420, damping: 20 }}
          >
            <Check size={11} strokeWidth={3} aria-hidden="true" />
          </motion.span>
        )}
      </AnimatePresence>

      <span className={styles.lineBody} data-memory={line.memory ? "" : undefined}>
        {/* Default (sync) mode: the entering text mounts in-flow and holds the
            line's box while the old text fades out as an absolute overlay — so a
            mid-swap frame never collapses the row (e.g. on a narrow card). */}
        <AnimatePresence initial={false}>
          {line.kind === "text" ? (
            <motion.span
              key={line.text}
              initial={{ opacity: 0, y: 6 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: -6, position: "absolute", top: 0, left: 0 }}
              transition={{ type: "spring", stiffness: 260, damping: 26 }}
            >
              <LineText line={line} />
            </motion.span>
          ) : (
            <motion.span key="fig-wrap" layout>
              <LineFigure line={line} />
            </motion.span>
          )}
        </AnimatePresence>
      </span>

      <AnimatePresence initial={false}>
        {line.memory && (
          <motion.span
            key="mem"
            className={styles.memoryTag}
            initial={{ opacity: 0, x: 6 }}
            animate={{ opacity: 1, x: 0 }}
            exit={{ opacity: 0, x: 6 }}
            transition={{ duration: 0.3 }}
          >
            memory · 100%
          </motion.span>
        )}
      </AnimatePresence>
    </motion.li>
  );
}

// ── Hero ─────────────────────────────────────────────────────────────────────

export default function HeroLoop({ onOpen }: HeroLoopProps): React.ReactElement {
  const prefersReduced = useReducedMotion();
  const reduced = prefersReduced === true;
  const [travel, setTravel] = useState(0);
  const [paused, setPaused] = useState(false);

  const idx = reduced ? N - 1 : travel % N;
  const scene: Scene = SCENES[idx];

  // Auto-advance, looping; paused on hover/focus and under reduced motion.
  useEffect(() => {
    if (reduced || paused) return;
    const t = setTimeout(() => setTravel((v) => v + 1), DWELL[idx] ?? 2600);
    return () => clearTimeout(t);
  }, [travel, idx, paused, reduced]);

  return (
    <MotionConfig reducedMotion="user">
      <div
        className={styles.root}
        style={{ "--stage": roleVar(scene.role) } as React.CSSProperties}
        onMouseEnter={() => setPaused(true)}
        onMouseLeave={() => setPaused(false)}
        onFocusCapture={() => setPaused(true)}
        onBlurCapture={() => setPaused(false)}
      >
        <div className={styles.kicker}>
          <span className={styles.kickerDot} aria-hidden="true" />
          The content loop
        </div>

        <Loop idx={idx} reduced={reduced} />

        {/* The slide — a single card the whole loop operates on. Opens the modal. */}
        <button
          type="button"
          className={styles.card}
          onClick={onOpen}
          aria-label="Open the interactive Try Kapi showcase"
        >
          <div className={styles.chrome}>
            <span className={styles.file}>
              <span className={styles.fileLocale}>{scene.file.split("/")[0]}</span>/
              {scene.file.split("/")[1]}
            </span>
            <span className={styles.badge}>{scene.stage}</span>
          </div>

          <ul className={styles.doc}>
            {scene.lines.map((line) => (
              <SlideLine key={line.id} line={line} />
            ))}
          </ul>

          {/* Footer swaps: formats fan in on Read, languages fan out on Ship. */}
          <div className={styles.footer}>
            <AnimatePresence mode="wait" initial={false}>
              {scene.key === "read" ? (
                <motion.div
                  key="formats"
                  className={styles.chipRow}
                  initial={{ opacity: 0 }}
                  animate={{ opacity: 1 }}
                  exit={{ opacity: 0 }}
                >
                  <span className={styles.chipLabel}>reads</span>
                  {READ_FORMATS.map((f, i) => (
                    <motion.span
                      key={f}
                      className={styles.chip}
                      initial={{ opacity: 0, y: 4 }}
                      animate={{ opacity: 1, y: 0 }}
                      transition={{ delay: 0.04 * i }}
                    >
                      {f}
                    </motion.span>
                  ))}
                  <span className={`${styles.chip} ${styles.chipMore}`}>50+</span>
                </motion.div>
              ) : scene.key === "ship" ? (
                <motion.div
                  key="langs"
                  className={styles.chipRow}
                  initial={{ opacity: 0 }}
                  animate={{ opacity: 1 }}
                  exit={{ opacity: 0 }}
                >
                  <span className={styles.chipLabel}>ships</span>
                  {SHIP_LANGS.map((l, i) => (
                    <motion.span
                      key={l}
                      className={`${styles.chip} ${styles.langChip}`}
                      data-active={l === "ja" ? "" : undefined}
                      initial={{ opacity: 0, scale: 0.7 }}
                      animate={{ opacity: 1, scale: 1 }}
                      transition={{ type: "spring", stiffness: 360, damping: 22, delay: 0.05 * i }}
                    >
                      {l}
                    </motion.span>
                  ))}
                  <span className={`${styles.chip} ${styles.chipMore}`}>…</span>
                </motion.div>
              ) : (
                <motion.p
                  key="cap"
                  className={styles.footHint}
                  initial={{ opacity: 0 }}
                  animate={{ opacity: 1 }}
                  exit={{ opacity: 0 }}
                >
                  {scene.key === "prep" && "Source settled · terms locked · secrets masked"}
                  {scene.key === "recycle" && "Only what changed is sent to translate"}
                  {scene.key === "translate" && "Inline tags & placeholders preserved"}
                  {scene.key === "check" && "Reviewed once · remembered everywhere"}
                </motion.p>
              )}
            </AnimatePresence>
          </div>
        </button>

        {/* Stage name + caption, crossfading per scene. */}
        <div className={styles.stageRow} aria-live="polite">
          <AnimatePresence mode="wait" initial={false}>
            <motion.p
              key={scene.key}
              className={styles.caption}
              initial={{ opacity: 0, y: 5 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: -5 }}
              transition={{ duration: 0.3 }}
            >
              <span className={styles.stageName}>{scene.stage}.</span> {scene.caption}
            </motion.p>
          </AnimatePresence>
        </div>

        <button type="button" className={styles.cta} onClick={onOpen}>
          Try Kapi in your browser
          <ArrowRight size={16} aria-hidden="true" />
        </button>
      </div>
    </MotionConfig>
  );
}
