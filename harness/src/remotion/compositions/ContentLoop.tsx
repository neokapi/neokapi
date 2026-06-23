import React from "react";
import {
  AbsoluteFill,
  interpolate,
  random,
  spring,
  useCurrentFrame,
  useVideoConfig,
} from "remotion";
import { inter, mono } from "../components/theme.ts";

// ─────────────────────────────────────────────────────────────────────────────
// ContentLoop — a cinematic, seamlessly-looping motion-graphics hero for the
// neokapi docs landing page. It is NOT a captured demo: it is a frame-rendered
// piece that leans into what only a rendered video can do (depth, parallax, a
// traveling light, particle bursts, morphs) to tell the product's "Content Loop"
// in miniature.
//
// The signature is the LOOP itself: a luminous orbital pipeline. A single document
// floats at the centre while a glowing playhead travels a great ring exactly once
// over the composition's length. The ring carries six stage nodes — Read · Prepare
// · Recycle · Translate · Check · Ship — and the playhead lights each one as its
// beat plays out on the central document: formats converge into one model, terms
// and a sensitive span are settled, recycled segments snap in from translation
// memory, language strings burst in to fill every line, a check sweep flips red to
// green, localized copies fan out — and then everything sweeps back into the
// centre as the camera pulls out to reveal the whole ring, and the loop closes.
//
// Seamless loop contract: every time-driven quantity is periodic over [0, FRAMES]
// or returns to its frame-0 value by FRAMES — the orbit angle wraps, the camera
// breathing is a sine of frame/FRAMES, the document dissolves back to its pre-Read
// scatter during the loop-back, and a short seam-fade at both ends hides any
// residual discontinuity. Rendered twice (dark + a refined light treatment) so the
// ThemedVideo light/dark toggle stays theme-matched to the docs page.
//
// Self-contained by design (only fonts are shared) so it can be reviewed and
// rendered independently of the demo pipeline.
// ─────────────────────────────────────────────────────────────────────────────

export const CL_FPS = 30;
export const CL_SIZE = 1080; // square — the loop motif reads centred, fits the hero aside slot
export const CL_FRAMES = 360; // 12s

const CX = CL_SIZE / 2;
const CY = CL_SIZE / 2;
const RING_R = 430;

export type ContentLoopMode = "light" | "dark";

export interface ContentLoopProps {
  themeMode?: ContentLoopMode;
  [key: string]: unknown;
}

// ── Palette ──────────────────────────────────────────────────────────────────
// Two accents only (brand teal + kapi orange) plus neutrals — restrained, premium.
// The dark treatment is deep-space navy; the light treatment is a refined paper
// white. Languages are tinted along the teal→orange axis (analogous, never rainbow).
interface Palette {
  bgInner: string;
  bgOuter: string;
  glowTeal: string;
  glowOrange: string;
  dust: string;
  ringBase: string;
  ringLit: string;
  panel: string;
  panelEdge: string;
  panelGlow: string;
  ink: string;
  inkDim: string;
  inkFaint: string;
  teal: string;
  orange: string;
  green: string;
  red: string;
  chipBg: string;
  chipEdge: string;
  redactBar: string;
  vignette: string;
  grain: number;
}

const DARK: Palette = {
  bgInner: "#0d1626",
  bgOuter: "#05080f",
  glowTeal: "rgba(37,194,160,0.30)",
  glowOrange: "rgba(255,122,69,0.20)",
  dust: "rgba(180,210,235,0.55)",
  ringBase: "rgba(150,180,210,0.14)",
  ringLit: "#36e3bf",
  panel: "rgba(17,26,42,0.92)",
  panelEdge: "rgba(160,200,235,0.16)",
  panelGlow: "rgba(54,227,191,0.30)",
  ink: "#eef3fb",
  inkDim: "#9fb0cc",
  inkFaint: "#5d6e8e",
  teal: "#36e3bf",
  orange: "#ff8a52",
  green: "#5ce6a0",
  red: "#ff7a8c",
  chipBg: "rgba(54,227,191,0.10)",
  chipEdge: "rgba(54,227,191,0.32)",
  redactBar: "#0a0f1a",
  vignette: "rgba(2,4,9,0.62)",
  grain: 0.05,
};

const LIGHT: Palette = {
  bgInner: "#ffffff",
  bgOuter: "#e9eef6",
  glowTeal: "rgba(20,168,136,0.18)",
  glowOrange: "rgba(232,97,42,0.13)",
  dust: "rgba(40,70,110,0.30)",
  ringBase: "rgba(40,70,110,0.12)",
  ringLit: "#12a888",
  panel: "rgba(255,255,255,0.96)",
  panelEdge: "rgba(20,40,70,0.12)",
  panelGlow: "rgba(20,168,136,0.26)",
  ink: "#142033",
  inkDim: "#55657f",
  inkFaint: "#92a0b6",
  teal: "#12a888",
  orange: "#e8612a",
  green: "#16915b",
  red: "#d23b50",
  chipBg: "rgba(18,168,136,0.10)",
  chipEdge: "rgba(18,168,136,0.34)",
  redactBar: "#1a2336",
  vignette: "rgba(180,195,220,0.30)",
  grain: 0.035,
};

// ── Math helpers ─────────────────────────────────────────────────────────────
const TAU = Math.PI * 2;
const deg = (d: number) => (d * Math.PI) / 180;
const polar = (r: number, angleDeg: number) => ({
  x: CX + r * Math.cos(deg(angleDeg)),
  y: CY + r * Math.sin(deg(angleDeg)),
});
/** Cubic ease-in-out. */
const ease = (t: number) => (t < 0.5 ? 4 * t * t * t : 1 - Math.pow(-2 * t + 2, 3) / 2);
/** Local 0..1 progress across a [start,end] window, clamped. */
const seg = (f: number, start: number, end: number) =>
  Math.max(0, Math.min(1, (f - start) / (end - start)));

// ── Stage plan ───────────────────────────────────────────────────────────────
// Six nodes evenly around the ring; the playhead reaches node i at frame i*STEP.
const STAGES = ["Read", "Prepare", "Recycle", "Translate", "Check", "Ship"] as const;
const STEP = CL_FRAMES / STAGES.length; // 60
const nodeAngle = (i: number) => -90 + i * (360 / STAGES.length); // node 0 at top

// Example document content (prose, not a code-owned count).
const DOC_TITLE = "Q4 Product Update";
const DOC_LINES = [
  "Revenue grew across every region.",
  "Onboarding now takes half the time.",
  "Enterprise contracts up to $4.2M.", // the redacted/sensitive line
  "Two new languages shipped this week.",
];
const FORMATS = [".pptx", ".docx", ".json", ".xliff", ".html", ".md", ".yaml", ".po"];
const LANGS = ["FR", "DE", "JA", "ES", "PT", "NL", "ZH", "AR", "KO", "IT"];
// Per-language sample renderings of the title, to make the translate/ship beats
// read as real content rather than placeholders.
const TITLE_BY_LANG: Record<string, string> = {
  FR: "Mise à jour T4",
  DE: "Q4-Update",
  JA: "第4四半期の更新",
  ES: "Novedades Q4",
  PT: "Atualização T4",
  NL: "Q4-update",
  ZH: "第四季度更新",
  AR: "تحديث الربع الرابع",
  KO: "4분기 업데이트",
  IT: "Aggiornamento Q4",
};

// ─────────────────────────────────────────────────────────────────────────────
// Background: gradient mesh + two parallax glows + drifting dust.
// ─────────────────────────────────────────────────────────────────────────────
const Background: React.FC<{ p: Palette; frame: number }> = ({ p, frame }) => {
  const t = frame / CL_FRAMES; // 0..1, periodic
  const drift = Math.sin(t * TAU);
  return (
    <AbsoluteFill>
      <AbsoluteFill
        style={{
          background: `radial-gradient(120% 120% at 50% 38%, ${p.bgInner} 0%, ${p.bgOuter} 72%)`,
        }}
      />
      {/* Two slow parallax glows that breathe with the loop. */}
      <AbsoluteFill
        style={{
          background: `radial-gradient(46% 46% at ${30 + drift * 6}% ${22 - drift * 4}%, ${p.glowTeal} 0%, transparent 60%)`,
        }}
      />
      <AbsoluteFill
        style={{
          background: `radial-gradient(50% 50% at ${72 - drift * 5}% ${82 + drift * 4}%, ${p.glowOrange} 0%, transparent 62%)`,
        }}
      />
      <Dust p={p} frame={frame} />
    </AbsoluteFill>
  );
};

const DUST_N = 70;
const Dust: React.FC<{ p: Palette; frame: number }> = ({ p, frame }) => {
  const t = frame / CL_FRAMES;
  return (
    <AbsoluteFill>
      {Array.from({ length: DUST_N }).map((_, i) => {
        const seed = i + 1;
        const bx = random(`dx${seed}`) * CL_SIZE;
        const by = random(`dy${seed}`) * CL_SIZE;
        const depth = 0.3 + random(`dz${seed}`) * 0.7; // parallax depth
        const ph = random(`dp${seed}`) * TAU;
        // Periodic drift so frame 0 == frame FRAMES.
        const x = bx + Math.sin(t * TAU + ph) * 26 * depth;
        const y = by + Math.cos(t * TAU + ph) * 20 * depth;
        const size = 1 + depth * 2.4;
        const tw = 0.25 + 0.55 * (0.5 + 0.5 * Math.sin(t * TAU * 2 + ph));
        return (
          <div
            key={i}
            style={{
              position: "absolute",
              left: x,
              top: y,
              width: size,
              height: size,
              borderRadius: size,
              background: p.dust,
              opacity: tw * depth,
              filter: depth > 0.7 ? "blur(0.4px)" : "blur(1.4px)",
            }}
          />
        );
      })}
    </AbsoluteFill>
  );
};

// ─────────────────────────────────────────────────────────────────────────────
// The orbital ring: the great loop with six stage nodes and a traveling playhead.
// ─────────────────────────────────────────────────────────────────────────────
const Ring: React.FC<{ p: Palette; frame: number }> = ({ p, frame }) => {
  const playAngle = -90 + (frame / CL_FRAMES) * 360;
  const head = polar(RING_R, playAngle);
  // The arc already traveled glows; render it as a conic-gradient stroke via SVG.
  const dash = TAU * RING_R;
  const progress = frame / CL_FRAMES;
  return (
    <AbsoluteFill>
      <svg width={CL_SIZE} height={CL_SIZE} style={{ position: "absolute", inset: 0 }}>
        <defs>
          <radialGradient id="headGlow" cx="50%" cy="50%" r="50%">
            <stop offset="0%" stopColor={p.ringLit} stopOpacity="0.9" />
            <stop offset="100%" stopColor={p.ringLit} stopOpacity="0" />
          </radialGradient>
          <linearGradient id="litArc" x1="0%" y1="0%" x2="100%" y2="0%">
            <stop offset="0%" stopColor={p.ringLit} stopOpacity="0" />
            <stop offset="100%" stopColor={p.ringLit} stopOpacity="1" />
          </linearGradient>
        </defs>
        {/* Base ring. */}
        <circle cx={CX} cy={CY} r={RING_R} fill="none" stroke={p.ringBase} strokeWidth={2} />
        {/* Lit trailing arc behind the playhead. */}
        <circle
          cx={CX}
          cy={CY}
          r={RING_R}
          fill="none"
          stroke={p.ringLit}
          strokeWidth={3}
          strokeLinecap="round"
          strokeDasharray={`${dash * 0.18} ${dash}`}
          strokeDashoffset={dash * (1 - progress) + dash * 0.18}
          transform={`rotate(-90 ${CX} ${CY})`}
          opacity={0.85}
          style={{ filter: `drop-shadow(0 0 10px ${p.ringLit})` }}
        />
      </svg>
      {/* Stage nodes. */}
      {STAGES.map((label, i) => (
        <Node key={label} p={p} i={i} label={label} frame={frame} playAngle={playAngle} />
      ))}
      {/* Playhead glow + core. */}
      <div
        style={{
          position: "absolute",
          left: head.x - 60,
          top: head.y - 60,
          width: 120,
          height: 120,
          borderRadius: 120,
          backgroundImage: `radial-gradient(circle, ${p.ringLit}55 0%, transparent 60%)`,
        }}
      />
      <div
        style={{
          position: "absolute",
          left: head.x - 7,
          top: head.y - 7,
          width: 14,
          height: 14,
          borderRadius: 14,
          background: p.ringLit,
          boxShadow: `0 0 18px 4px ${p.ringLit}`,
        }}
      />
    </AbsoluteFill>
  );
};

const Node: React.FC<{
  p: Palette;
  i: number;
  label: string;
  frame: number;
  playAngle: number;
}> = ({ p, i, label, frame, playAngle }) => {
  const a = nodeAngle(i);
  const pos = polar(RING_R, a);
  // How close (in frames) is the playhead to this node's beat centre?
  const center = i * STEP;
  let d = Math.abs(frame - center);
  d = Math.min(d, CL_FRAMES - d); // wrap distance
  const lit = interpolate(d, [0, 26], [1, 0], { extrapolateRight: "clamp" });
  const size = 12 + lit * 8;
  // Label sits outside the ring, anchored by quadrant so it never overlaps the node.
  const out = polar(RING_R + 30, a);
  const right = pos.x >= CX;
  return (
    <>
      <div
        style={{
          position: "absolute",
          left: pos.x - size / 2,
          top: pos.y - size / 2,
          width: size,
          height: size,
          borderRadius: size,
          background: lit > 0.04 ? p.ringLit : p.bgInner,
          border: `2px solid ${p.ringLit}`,
          boxShadow: lit > 0.04 ? `0 0 ${10 + lit * 22}px ${2 + lit * 4}px ${p.ringLit}` : "none",
          opacity: 0.5 + lit * 0.5,
        }}
      />
      <div
        style={{
          position: "absolute",
          left: right ? out.x : undefined,
          right: right ? undefined : CL_SIZE - out.x,
          top: out.y - 12,
          fontFamily: mono.fontFamily,
          fontSize: 19,
          fontWeight: 600,
          letterSpacing: 0.5,
          color: lit > 0.2 ? p.ink : p.inkFaint,
          opacity: 0.6 + lit * 0.4,
          textShadow: lit > 0.4 ? `0 0 14px ${p.ringLit}` : "none",
          whiteSpace: "nowrap",
          transition: "none",
        }}
      >
        {label}
      </div>
    </>
  );
};

// ─────────────────────────────────────────────────────────────────────────────
// The central document — a stylized page that morphs through each beat.
// `state` is derived from the frame; the card itself is persistent.
// ─────────────────────────────────────────────────────────────────────────────
const DOC_W = 460;
const DOC_H = 540;

const DocumentCard: React.FC<{ p: Palette; frame: number }> = ({ p, frame }) => {
  const { fps } = useVideoConfig();

  // Card materialization: it assembles during Read (8→48) and dissolves during the
  // loop-back (320→358) back to nothing, so frame 0 == frame 360.
  const assemble = seg(frame, 8, 46);
  // Dissolve earlier/faster so the loop-back wordmark gets a clear stage, and the
  // page is fully gone by FRAMES (== frame 0) for a seamless loop.
  const dissolve = 1 - seg(frame, 304, 338);
  const cardOpacity = Math.min(assemble, dissolve);
  const cardScale = 0.9 + 0.1 * ease(assemble) - 0.06 * (1 - dissolve);

  // Per-line target language during Translate/Ship: cycle the visible language so
  // the doc reads as "filling every language".
  const langIdx = Math.floor(interpolate(frame, [180, 312], [0, LANGS.length - 0.01], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  }));
  const lang = LANGS[Math.max(0, Math.min(LANGS.length - 1, langIdx))];

  // Beat flags.
  const termsLocal = seg(frame, 60, 96); // Prepare: term pills + brand ring
  const redactLocal = seg(frame, 72, 104); // Prepare: redaction bar sweep
  const recycleLocal = seg(frame, 120, 156);
  const translateLocal = seg(frame, 168, 224);
  const checkLocal = seg(frame, 234, 268);
  const shipLocal = seg(frame, 288, 320);

  const cardFloat = Math.sin((frame / CL_FRAMES) * TAU) * 6;

  return (
    <div
      style={{
        position: "absolute",
        left: CX - DOC_W / 2,
        top: CY - DOC_H / 2 + cardFloat,
        width: DOC_W,
        height: DOC_H,
        opacity: cardOpacity,
        transform: `scale(${cardScale})`,
        transformOrigin: "center",
      }}
    >
      {/* Glow halo under the card. */}
      <div
        style={{
          position: "absolute",
          inset: -28,
          borderRadius: 36,
          background: `radial-gradient(60% 55% at 50% 42%, ${p.panelGlow} 0%, transparent 70%)`,
          filter: "blur(8px)",
          opacity: 0.7 + translateLocal * 0.3,
        }}
      />
      {/* The page. */}
      <div
        style={{
          position: "absolute",
          inset: 0,
          borderRadius: 22,
          background: p.panel,
          border: `1px solid ${p.panelEdge}`,
          boxShadow: `0 40px 90px rgba(0,0,0,0.45), inset 0 1px 0 ${p.panelEdge}`,
          overflow: "hidden",
          backdropFilter: "blur(2px)",
        }}
      >
        {/* Chrome bar: filename + locale stamp. */}
        <div
          style={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            padding: "16px 20px",
            borderBottom: `1px solid ${p.panelEdge}`,
          }}
        >
          <span style={{ fontFamily: mono.fontFamily, fontSize: 15, color: p.inkFaint }}>
            update.pptx
          </span>
          <LocaleStamp p={p} frame={frame} lang={lang} translateLocal={translateLocal} />
        </div>

        {/* Title line. */}
        <div style={{ padding: "26px 24px 6px" }}>
          <DocTitle
            p={p}
            frame={frame}
            translateLocal={translateLocal}
            shipLocal={shipLocal}
            lang={lang}
          />
        </div>

        {/* Body lines. */}
        <div style={{ padding: "10px 24px", display: "flex", flexDirection: "column", gap: 18 }}>
          {DOC_LINES.map((text, idx) => (
            <DocLine
              key={idx}
              p={p}
              idx={idx}
              text={text}
              frame={frame}
              fps={fps}
              termsLocal={termsLocal}
              redactLocal={redactLocal}
              recycleLocal={recycleLocal}
              translateLocal={translateLocal}
              checkLocal={checkLocal}
              lang={lang}
            />
          ))}
        </div>

        {/* Brand-voice ring pulse during Prepare. */}
        <BrandPulse p={p} local={termsLocal} />
      </div>
    </div>
  );
};

const LocaleStamp: React.FC<{ p: Palette; frame: number; lang: string; translateLocal: number }> = ({
  p,
  lang,
  translateLocal,
}) => {
  const showLang = translateLocal > 0.08;
  return (
    <span
      style={{
        fontFamily: mono.fontFamily,
        fontSize: 14,
        fontWeight: 600,
        padding: "3px 9px",
        borderRadius: 999,
        color: showLang ? p.teal : p.inkFaint,
        background: showLang ? p.chipBg : "transparent",
        border: `1px solid ${showLang ? p.chipEdge : "transparent"}`,
        minWidth: 34,
        textAlign: "center",
      }}
    >
      {showLang ? lang : "EN"}
    </span>
  );
};

const DocTitle: React.FC<{
  p: Palette;
  frame: number;
  translateLocal: number;
  shipLocal: number;
  lang: string;
}> = ({ p, frame, translateLocal, lang }) => {
  const translated = translateLocal > 0.12;
  const text = translated ? (TITLE_BY_LANG[lang] ?? DOC_TITLE) : DOC_TITLE;
  // A subtle vertical "roll" each time the language flips.
  const flip = Math.sin(interpolate(frame, [180, 312], [0, TAU * LANGS.length])) * (translated ? 1.5 : 0);
  return (
    <div
      style={{
        fontFamily: inter.fontFamily,
        fontSize: 30,
        fontWeight: 720,
        letterSpacing: -0.4,
        color: p.ink,
        lineHeight: 1.1,
        transform: `translateY(${flip}px)`,
      }}
    >
      {text}
    </div>
  );
};

const DocLine: React.FC<{
  p: Palette;
  idx: number;
  text: string;
  frame: number;
  fps: number;
  termsLocal: number;
  redactLocal: number;
  recycleLocal: number;
  translateLocal: number;
  checkLocal: number;
  lang: string;
}> = ({ p, idx, text, frame, fps, termsLocal, redactLocal, recycleLocal, translateLocal, checkLocal, lang }) => {
  const isSensitive = idx === 2;
  const isChanged = idx === 3; // the "changed" line that TM can't recycle → translated fresh

  // All lines write in (staggered) as the page assembles during Read — so the
  // document never reads sparse. Lines 0–2 are "recyclable from memory"; the
  // changed line waits for a fresh translation.
  const recycledLine = idx === 0 || idx === 1;
  const appear = spring({ frame: frame - (12 + idx * 6), fps, config: { damping: 200 } });
  // Recycle: the recyclable lines get a brief left-snap + "from memory" tag.
  const fromMemory = recycledLine && recycleLocal > 0.05 && translateLocal < 0.2;
  const reSnap = recycledLine
    ? interpolate(frame, [116 + idx * 8, 126 + idx * 8], [1, 0], {
        extrapolateLeft: "clamp",
        extrapolateRight: "clamp",
      })
    : 0;
  const memFlash = fromMemory ? interpolate(recycleLocal, [0, 0.2, 0.85, 1], [0, 1, 1, 0]) : 0;

  // Translate fill: each line gets "filled" as the language burst lands (staggered).
  const fillStart = 170 + idx * 9;
  const fill = interpolate(frame, [fillStart, fillStart + 16], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  const translated = translateLocal > 0.1 && fill > 0.2;

  // Width pseudo-randomized per line for a realistic ragged paragraph.
  const w = [88, 72, 64, 80][idx];

  // Check sweep: a scan passes top→bottom; this line resolves red→green as it passes.
  const sweepY = checkLocal; // 0..1 over the card height
  const lineFrac = (idx + 1) / (DOC_LINES.length + 1);
  const passed = sweepY > lineFrac;
  const showCheck = checkLocal > 0.04 && passed;

  return (
    <div
      style={{
        position: "relative",
        opacity: appear,
        transform: `translateX(${interpolate(appear, [0, 1], [-12, 0]) + reSnap * -18}px)`,
      }}
    >
      {/* Memory-recall highlight wash during Recycle. */}
      {memFlash > 0.01 && (
        <div
          style={{
            position: "absolute",
            inset: "-4px -10px",
            borderRadius: 8,
            background: p.chipBg,
            opacity: memFlash * 0.8,
            pointerEvents: "none",
          }}
        />
      )}
      {/* The text bar / glyph row. */}
      <div style={{ display: "flex", alignItems: "center", gap: 8, position: "relative" }}>
        <span
          style={{
            fontFamily: inter.fontFamily,
            fontSize: 17,
            color: translated ? p.ink : p.inkDim,
            opacity: isChanged && !translated ? 0.32 : 1,
            // The changed line shows a dim placeholder until translated.
          }}
        >
          {renderLineText({ text, translated, translateLocal, lang, isChanged })}
        </span>
      </div>

      {/* Underline bar to suggest a measured line length (premium, abstract). */}
      <div
        style={{
          marginTop: 7,
          height: 5,
          width: `${w}%`,
          borderRadius: 5,
          background: translated
            ? `linear-gradient(90deg, ${p.teal}, ${p.orange})`
            : p.panelEdge,
          opacity: translated ? 0.6 : 0.5,
          transformOrigin: "left",
          transform: `scaleX(${translated ? fill : 1})`,
        }}
      />

      {/* Term pills on line 1 during Prepare. */}
      {idx === 0 && termsLocal > 0.1 && translateLocal < 0.1 && (
        <TermPills p={p} local={termsLocal} />
      )}

      {/* Sensitive span → redaction bar sweeps across during Prepare, then holds
          (redactLocal saturates at 1 and stays, so the figure is never revealed). */}
      {isSensitive && redactLocal > 0.02 && <RedactBar p={p} local={redactLocal} />}

      {/* "from memory" tag during Recycle. */}
      {fromMemory && (
        <span
          style={{
            position: "absolute",
            right: -6,
            top: -4,
            fontFamily: mono.fontFamily,
            fontSize: 11,
            fontWeight: 600,
            color: p.teal,
            background: p.chipBg,
            border: `1px solid ${p.chipEdge}`,
            borderRadius: 999,
            padding: "1px 7px",
            opacity: interpolate(recycleLocal, [0, 0.2, 0.85, 1], [0, 1, 1, 0]),
          }}
        >
          from memory
        </span>
      )}

      {/* Check mark when the sweep has passed. */}
      {showCheck && (
        <span
          style={{
            position: "absolute",
            right: -2,
            top: 0,
            color: p.green,
            fontSize: 16,
            fontWeight: 800,
            opacity: interpolate(checkLocal, [lineFrac, lineFrac + 0.12], [0, 1], {
              extrapolateLeft: "clamp",
              extrapolateRight: "clamp",
            }),
            textShadow: `0 0 10px ${p.green}`,
          }}
        >
          ✓
        </span>
      )}
    </div>
  );
};

function renderLineText(o: {
  text: string;
  translated: boolean;
  translateLocal: number;
  lang: string;
  isChanged: boolean;
}): string {
  if (o.isChanged && o.translateLocal < 0.1) return "—";
  if (!o.translated) return o.text;
  // A light pseudo-localization: keep it abstract (we are not claiming real MT in
  // a hero), shown as the source dimmed under the accent fill bar. Keep the source
  // words — the colour + fill bar carry the "now in <lang>" meaning, no fake strings.
  return o.text;
}

const TermPills: React.FC<{ p: Palette; local: number }> = ({ p, local }) => {
  const pills = ["region", "onboarding"];
  return (
    <div style={{ display: "flex", gap: 8, marginTop: 8 }}>
      {pills.map((t, i) => {
        const o = interpolate(local, [0.05 + i * 0.08, 0.25 + i * 0.08, 0.85, 1], [0, 1, 1, 0], {
          extrapolateLeft: "clamp",
          extrapolateRight: "clamp",
        });
        return (
          <span
            key={t}
            style={{
              fontFamily: mono.fontFamily,
              fontSize: 12,
              color: p.orange,
              background: `${p.orange}1a`,
              border: `1px solid ${p.orange}55`,
              borderRadius: 6,
              padding: "1px 7px",
              opacity: o,
              transform: `translateY(${interpolate(o, [0, 1], [6, 0])}px)`,
            }}
          >
            {t}
          </span>
        );
      })}
    </div>
  );
};

const RedactBar: React.FC<{ p: Palette; local: number }> = ({ p, local }) => {
  // Sweeps across the sensitive number, then holds as a solid censor bar.
  const sweep = ease(Math.min(1, local));
  return (
    <div
      style={{
        position: "absolute",
        right: 0,
        top: 0,
        height: 22,
        width: `${52 * sweep}%`,
        background: p.redactBar,
        borderRadius: 4,
        boxShadow: `0 0 0 1px ${p.panelEdge}`,
      }}
    />
  );
};

const BrandPulse: React.FC<{ p: Palette; local: number }> = ({ p, local }) => {
  if (local <= 0.01) return null;
  const r = interpolate(local, [0, 1], [0.6, 1.18]);
  const o = interpolate(local, [0, 0.3, 1], [0, 0.5, 0]);
  return (
    <div
      style={{
        position: "absolute",
        inset: 0,
        borderRadius: 22,
        border: `2px solid ${p.orange}`,
        transform: `scale(${r})`,
        opacity: o,
        pointerEvents: "none",
      }}
    />
  );
};

// ─────────────────────────────────────────────────────────────────────────────
// Format chips converging into the document during Read (the "any format → one
// model" beat). They start out on the ring and fly to the card centre.
// ─────────────────────────────────────────────────────────────────────────────
const FormatConverge: React.FC<{ p: Palette; frame: number }> = ({ p, frame }) => {
  const local = seg(frame, 2, 50);
  if (local >= 1) return null;
  return (
    <AbsoluteFill>
      {FORMATS.map((f, i) => {
        const a = (i / FORMATS.length) * 360 - 90;
        const start = polar(RING_R - 8, a);
        const t = ease(Math.max(0, Math.min(1, (local - i * 0.04) / 0.55)));
        const x = interpolate(t, [0, 1], [start.x, CX]);
        const y = interpolate(t, [0, 1], [start.y, CY]);
        const o = interpolate(t, [0, 0.15, 0.8, 1], [0, 1, 1, 0]);
        const s = interpolate(t, [0, 1], [1, 0.4]);
        return (
          <div
            key={f}
            style={{
              position: "absolute",
              left: x,
              top: y,
              transform: `translate(-50%,-50%) scale(${s})`,
              fontFamily: mono.fontFamily,
              fontSize: 18,
              fontWeight: 600,
              color: p.teal,
              background: p.chipBg,
              border: `1px solid ${p.chipEdge}`,
              borderRadius: 8,
              padding: "4px 10px",
              opacity: o,
              boxShadow: `0 0 16px ${p.panelGlow}`,
              whiteSpace: "nowrap",
            }}
          >
            {f}
          </div>
        );
      })}
    </AbsoluteFill>
  );
};

// ─────────────────────────────────────────────────────────────────────────────
// Translation memory "sink": a reservoir above the card that recycled segments
// pour out of during Recycle, snapping toward the document.
// ─────────────────────────────────────────────────────────────────────────────
const MemorySink: React.FC<{ p: Palette; frame: number }> = ({ p, frame }) => {
  const local = seg(frame, 110, 162);
  if (local <= 0 || local >= 1) return null;
  const tankY = CY - DOC_H / 2 - 96;
  const o = interpolate(local, [0, 0.12, 0.85, 1], [0, 1, 1, 0]);
  return (
    <AbsoluteFill style={{ opacity: o }}>
      {/* Tank. */}
      <div
        style={{
          position: "absolute",
          left: CX - 70,
          top: tankY,
          width: 140,
          height: 46,
          borderRadius: 12,
          background: p.chipBg,
          border: `1px solid ${p.chipEdge}`,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          fontFamily: mono.fontFamily,
          fontSize: 15,
          fontWeight: 700,
          color: p.teal,
          letterSpacing: 1,
          boxShadow: `0 0 26px ${p.panelGlow}`,
        }}
      >
        TM ·  memory
      </div>
      {/* Segments dropping from the tank into the card. */}
      {Array.from({ length: 5 }).map((_, i) => {
        const t = (local * 1.8 + i * 0.18) % 1;
        const y = interpolate(t, [0, 1], [tankY + 46, CY - DOC_H / 2 + 70]);
        const op = interpolate(t, [0, 0.1, 0.8, 1], [0, 1, 1, 0]);
        const wob = Math.sin(t * 8 + i) * 10;
        return (
          <div
            key={i}
            style={{
              position: "absolute",
              left: CX - 26 + wob,
              top: y,
              width: 52,
              height: 7,
              borderRadius: 7,
              background: `linear-gradient(90deg, ${p.teal}, transparent)`,
              opacity: op,
              boxShadow: `0 0 10px ${p.teal}`,
            }}
          />
        );
      })}
    </AbsoluteFill>
  );
};

// ─────────────────────────────────────────────────────────────────────────────
// Language burst: language tokens shoot from their angle on the ring into the
// document during Translate — the hero beat. Glowing trails.
// ─────────────────────────────────────────────────────────────────────────────
const LanguageBurst: React.FC<{ p: Palette; frame: number }> = ({ p, frame }) => {
  const local = seg(frame, 160, 226);
  if (local <= 0 || local >= 1) return null;
  return (
    <AbsoluteFill>
      {LANGS.map((code, i) => {
        const a = (i / LANGS.length) * 360 - 90 + 18;
        const start = polar(RING_R - 20, a);
        // Staggered launch; each flies in, then fades just inside the card edge.
        const t = ease(Math.max(0, Math.min(1, (local - i * 0.05) / 0.4)));
        const tin = polar(DOC_W * 0.42, a); // land near the card edge
        const x = interpolate(t, [0, 1], [start.x, tin.x]);
        const y = interpolate(t, [0, 1], [start.y, tin.y]);
        const o = interpolate(t, [0, 0.12, 0.7, 1], [0, 1, 1, 0]);
        const tint = mixTealOrange(p, i / (LANGS.length - 1));
        // Motion-blur trail: a short streak opposite the travel direction.
        const dx = (tin.x - start.x) * 0.05;
        const dy = (tin.y - start.y) * 0.05;
        const ang = (Math.atan2(dy, dx) * 180) / Math.PI;
        const speed = 1 - Math.abs(t - 0.4) * 1.6;
        return (
          <React.Fragment key={code}>
            <div
              style={{
                position: "absolute",
                left: x,
                top: y,
                width: 40 + Math.max(0, speed) * 70,
                height: 3,
                transform: `translate(-100%,-50%) rotate(${ang}deg)`,
                transformOrigin: "right center",
                background: `linear-gradient(90deg, transparent, ${tint})`,
                opacity: o * 0.7,
                borderRadius: 3,
              }}
            />
            <div
              style={{
                position: "absolute",
                left: x,
                top: y,
                transform: "translate(-50%,-50%)",
                fontFamily: mono.fontFamily,
                fontSize: 18,
                fontWeight: 700,
                color: tint,
                opacity: o,
                textShadow: `0 0 12px ${tint}`,
                whiteSpace: "nowrap",
              }}
            >
              {code}
            </div>
          </React.Fragment>
        );
      })}
    </AbsoluteFill>
  );
};

function mixTealOrange(p: Palette, t: number): string {
  // Blend between teal and orange in sRGB — analogous, premium.
  const a = hexToRgb(p.teal);
  const b = hexToRgb(p.orange);
  const r = Math.round(a[0] + (b[0] - a[0]) * t);
  const g = Math.round(a[1] + (b[1] - a[1]) * t);
  const bl = Math.round(a[2] + (b[2] - a[2]) * t);
  return `rgb(${r},${g},${bl})`;
}
function hexToRgb(hex: string): [number, number, number] {
  const h = hex.replace("#", "");
  return [parseInt(h.slice(0, 2), 16), parseInt(h.slice(2, 4), 16), parseInt(h.slice(4, 6), 16)];
}

// ─────────────────────────────────────────────────────────────────────────────
// Ship fan-out: localized copies of the card flash outward along the ring during
// Ship, then sweep back into the centre on the loop-back.
// ─────────────────────────────────────────────────────────────────────────────
const SHIP_LANGS = ["FR", "DE", "JA", "ES", "ZH"];
const ShipFan: React.FC<{ p: Palette; frame: number }> = ({ p, frame }) => {
  const local = seg(frame, 286, 356); // fan out then back (covers loop-back)
  if (local <= 0) return null;
  return (
    <AbsoluteFill>
      {SHIP_LANGS.map((code, i) => {
        const spread = (i - (SHIP_LANGS.length - 1) / 2) * 26; // degrees around top
        const a = -90 + spread;
        // Out (0→0.5) along the ring, hold, then back in (0.7→1) to centre.
        const outT = ease(Math.min(1, local / 0.45));
        const backT = ease(Math.max(0, (local - 0.62) / 0.38));
        const rOut = interpolate(outT, [0, 1], [DOC_W * 0.2, RING_R - 40]);
        const r = interpolate(backT, [0, 1], [rOut, 0]);
        const pos = polar(r, a);
        const o =
          interpolate(local, [0, 0.08, 0.62, 1], [0, 1, 1, 0]) *
          interpolate(backT, [0, 1], [1, 0.2]);
        const scale = interpolate(r, [0, RING_R], [0.5, 1]) * 0.9;
        return (
          <div
            key={code}
            style={{
              position: "absolute",
              left: pos.x,
              top: pos.y,
              transform: `translate(-50%,-50%) scale(${scale})`,
              width: 116,
              height: 138,
              borderRadius: 12,
              background: p.panel,
              border: `1px solid ${p.panelEdge}`,
              boxShadow: `0 18px 40px rgba(0,0,0,0.45), 0 0 22px ${p.panelGlow}`,
              opacity: o,
              padding: 12,
              overflow: "hidden",
            }}
          >
            <div
              style={{
                fontFamily: mono.fontFamily,
                fontSize: 12,
                fontWeight: 700,
                color: p.teal,
                marginBottom: 8,
              }}
            >
              {code}
            </div>
            <div style={{ fontFamily: inter.fontFamily, fontSize: 13, fontWeight: 700, color: p.ink, lineHeight: 1.15 }}>
              {TITLE_BY_LANG[code]}
            </div>
            {[70, 88, 60].map((w, k) => (
              <div
                key={k}
                style={{
                  marginTop: 9,
                  height: 4,
                  width: `${w}%`,
                  borderRadius: 4,
                  background: `linear-gradient(90deg, ${p.teal}, ${p.orange})`,
                  opacity: 0.55,
                }}
              />
            ))}
          </div>
        );
      })}
    </AbsoluteFill>
  );
};

// ─────────────────────────────────────────────────────────────────────────────
// Wordmark + tagline — resolves quietly at the loop-back when the camera pulls
// out to reveal the whole ring, then fades for the seam. Restrained, no
// superlatives, echoing the site's own voice.
// ─────────────────────────────────────────────────────────────────────────────
const Wordmark: React.FC<{ p: Palette; frame: number }> = ({ p, frame }) => {
  // Visible only briefly at the loop-back reveal.
  const o = interpolate(frame, [300, 322, 348, 358], [0, 1, 1, 0], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  if (o <= 0.01) return null;
  const rise = interpolate(o, [0, 1], [10, 0]);
  return (
    <AbsoluteFill style={{ justifyContent: "center", alignItems: "center" }}>
      <div style={{ textAlign: "center", opacity: o, transform: `translateY(${rise}px)` }}>
        <div
          style={{
            fontFamily: mono.fontFamily,
            fontSize: 24,
            fontWeight: 700,
            letterSpacing: 2,
            color: p.teal,
            textShadow: `0 0 18px ${p.ringLit}`,
          }}
        >
          kapi
        </div>
        <div
          style={{
            fontFamily: inter.fontFamily,
            fontSize: 27,
            fontWeight: 600,
            color: p.ink,
            marginTop: 12,
            letterSpacing: -0.2,
          }}
        >
          Get it right. Then get it everywhere.
        </div>
      </div>
    </AbsoluteFill>
  );
};

// ─────────────────────────────────────────────────────────────────────────────
// Grain + vignette + loop-seam fade (foreground polish).
// ─────────────────────────────────────────────────────────────────────────────
const Grain: React.FC<{ p: Palette }> = ({ p }) => {
  // A static SVG fractal-noise overlay — cheap film grain that adds texture.
  const svg = encodeURIComponent(
    `<svg xmlns='http://www.w3.org/2000/svg' width='180' height='180'><filter id='n'><feTurbulence type='fractalNoise' baseFrequency='0.9' numOctaves='2' stitchTiles='stitch'/></filter><rect width='100%' height='100%' filter='url(%23n)'/></svg>`.replace(
      /#/g,
      "%23",
    ),
  );
  return (
    <AbsoluteFill
      style={{
        backgroundImage: `url("data:image/svg+xml,${svg}")`,
        opacity: p.grain,
        mixBlendMode: "overlay",
        pointerEvents: "none",
      }}
    />
  );
};

const Vignette: React.FC<{ p: Palette }> = ({ p }) => (
  <AbsoluteFill
    style={{
      background: `radial-gradient(120% 120% at 50% 50%, transparent 56%, ${p.vignette} 100%)`,
      pointerEvents: "none",
    }}
  />
);

// ─────────────────────────────────────────────────────────────────────────────
// Composition root.
// ─────────────────────────────────────────────────────────────────────────────
export const ContentLoop: React.FC<ContentLoopProps> = ({ themeMode = "dark" }) => {
  const frame = useCurrentFrame();
  const p = themeMode === "light" ? LIGHT : DARK;

  // Camera: gentle periodic breathing + a pull-back reveal at the loop-back
  // (returns to the frame-0 value at FRAMES so the loop is seamless).
  const breathe = 1 + 0.015 * Math.sin((frame / CL_FRAMES) * TAU);
  const pullBack = interpolate(frame, [296, 330, 360], [0, -0.16, 0], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  const camScale = breathe + pullBack;
  const camRot = 0.6 * Math.sin((frame / CL_FRAMES) * TAU); // tiny dutch tilt

  // Seam pulse: the scene is already near-continuous at the loop point (empty ring,
  // comet-tail and playhead both at the top, no document), so we need only a gentle
  // dim — not a full blackout — to smooth any residual edge. A shallow breath reads
  // as a deliberate cinematic beat rather than a jarring cut.
  const seam = Math.min(
    interpolate(frame, [0, 7], [0, 1], { extrapolateRight: "clamp" }),
    interpolate(frame, [CL_FRAMES - 7, CL_FRAMES], [1, 0], { extrapolateLeft: "clamp" }),
  );
  const seamDim = (1 - seam) * 0.5;

  return (
    <AbsoluteFill style={{ background: p.bgOuter, fontFamily: inter.fontFamily }}>
      {/* Background sits below the camera transform for a parallax feel. */}
      <Background p={p} frame={frame} />

      <AbsoluteFill
        style={{
          transform: `scale(${camScale}) rotate(${camRot}deg)`,
          transformOrigin: "center",
        }}
      >
        <Ring p={p} frame={frame} />
        <MemorySink p={p} frame={frame} />
        <DocumentCard p={p} frame={frame} />
        <FormatConverge p={p} frame={frame} />
        <LanguageBurst p={p} frame={frame} />
        <ShipFan p={p} frame={frame} />
      </AbsoluteFill>

      <Wordmark p={p} frame={frame} />
      <Vignette p={p} />
      <Grain p={p} />
      {/* Seam pulse overlay. */}
      <AbsoluteFill style={{ background: p.bgOuter, opacity: seamDim, pointerEvents: "none" }} />
    </AbsoluteFill>
  );
};
