import React, { useRef } from "react";
import { ArrowRight } from "lucide-react";
import gsap from "gsap";
import { useGSAP } from "@gsap/react";
import { DrawSVGPlugin } from "gsap/DrawSVGPlugin";
import { MotionPathPlugin } from "gsap/MotionPathPlugin";
import s from "./storyboard.module.css";

// The landing centerpiece: a hand-drawn marker-on-whiteboard storyboard of kapi's
// two interlocking loops, choreographed with GSAP. Nothing here boots the WASM
// engine — it is pure SVG + timeline — so the page stays zero-wasm on load; the
// CTA (and the board itself) calls `onOpen` to mount the live demo modal, exactly
// as the old hero did.
//
//   LOOP 1 — THE CONTENT LOOP (monolingual): PREP → WRITE ⇄ CHECK → SHIP.
//            The write⇄check back-and-forth is drawn as a little return arrow.
//   LOOP 2 — GOING MULTILINGUAL: READ → PREP → RECYCLE → TRANSLATE → CHECK → SHIP,
//            closed by the signature big return loop back to READ.
//   SHIP (loop 1) feeds READ (loop 2): one English source, every language back.
//
// The whole performance is one GSAP timeline: DrawSVG draws the marker arrows and
// the board frame; MotionPath flies the "recycle" memory segments and the
// "translate" language chips along curved paths. Colour is never animated — it is
// all CSS vars — so the board re-themes (light whiteboard / dark slate) for free.
//
// SSR/SSG: the SVG renders fully visible by default, so the statically-rendered
// HTML (and any no-JS reader) shows the finished board. useGSAP runs only on the
// client, in an isomorphic layout effect, so it sets the hidden "from" states
// before first paint — no flash. prefers-reduced-motion: we skip the timeline
// entirely and leave the board in its drawn end-state (particles stay hidden).

interface HeroStoryboardProps {
  /** Open the live demo modal. */
  onOpen: () => void;
}

// ── Geometry helpers (pure, deterministic → safe at SSR/render time) ──────────
const rad = (d: number): number => (d * Math.PI) / 180;
const f = (n: number): number => Math.round(n * 10) / 10;

/** A V arrowhead at (ex,ey), pointing along the direction from (px,py). */
function head(px: number, py: number, ex: number, ey: number, size = 9.5): string {
  const a = Math.atan2(ey - py, ex - px);
  return (
    `M ${f(ex + size * Math.cos(a + rad(150)))} ${f(ey + size * Math.sin(a + rad(150)))} ` +
    `L ${f(ex)} ${f(ey)} ` +
    `L ${f(ex + size * Math.cos(a - rad(150)))} ${f(ey + size * Math.sin(a - rad(150)))}`
  );
}

/** A short marker arrow (x1,y1)→(x2,y2) with an optional perpendicular bend. */
function arrow(x1: number, y1: number, x2: number, y2: number, bend = 0): string {
  const mx = (x1 + x2) / 2;
  const my = (y1 + y2) / 2;
  const len = Math.hypot(x2 - x1, y2 - y1) || 1;
  const cx = mx + (-(y2 - y1) / len) * bend;
  const cy = my + ((x2 - x1) / len) * bend;
  return `M ${f(x1)} ${f(y1)} Q ${f(cx)} ${f(cy)} ${f(x2)} ${f(y2)} ${head(cx, cy, x2, y2)}`;
}

/** A cubic marker arrow with an arrowhead at the end. */
function cubic(
  x1: number,
  y1: number,
  c1x: number,
  c1y: number,
  c2x: number,
  c2y: number,
  x2: number,
  y2: number,
): string {
  return (
    `M ${f(x1)} ${f(y1)} C ${f(c1x)} ${f(c1y)} ${f(c2x)} ${f(c2y)} ${f(x2)} ${f(y2)} ` +
    head(c2x, c2y, x2, y2)
  );
}

// ── Layout (SVG user space; viewBox 0 0 560 560) ──────────────────────────────
const A1_W = 100;
const A1_H = 46;
const A1_TOP = 118;
const A1_CY = A1_TOP + A1_H / 2;
const A1 = [
  { id: "prep", label: "PREP", sub: "terms · brand", cx: 90 },
  { id: "write", label: "WRITE", sub: "draft & edit", cx: 216.7 },
  { id: "check", label: "CHECK", sub: "lint · tone · terms", cx: 343.3 },
  { id: "ship", label: "SHIP", sub: "English ✓", cx: 470, accent: true },
];

const A2_W = 148;
const A2_H = 50;
const A2 = [
  {
    id: "read",
    label: "READ",
    sub: "any format pours in",
    cx: 114,
    cy: 355,
    hook: "hbBoxRead",
    subHook: "hbSubRead",
  },
  {
    id: "prep2",
    label: "PREP",
    sub: "redact · protect · shape",
    cx: 280,
    cy: 355,
    green: true,
    hook: "hbBoxPrep2",
    subHook: "hbSubPrep2",
  },
  {
    id: "recycle",
    label: "RECYCLE",
    sub: "reuse from memory",
    cx: 446,
    cy: 355,
    green: true,
    hook: "hbBoxRecycle",
    subHook: "hbSubRecycle",
  },
  {
    id: "translate",
    label: "TRANSLATE",
    sub: "AI fills every language",
    cx: 446,
    cy: 477,
    green: true,
    hook: "hbBoxTranslate",
    subHook: "hbSubTranslate",
  },
  {
    id: "check2",
    label: "CHECK",
    sub: "red-line / green-line",
    cx: 280,
    cy: 477,
    hook: "hbBoxCheck2",
    subHook: "hbSubCheck2",
  },
  {
    id: "ship2",
    label: "SHIP",
    sub: "every language · file",
    cx: 114,
    cy: 477,
    accent: true,
    hook: "hbBoxShip2",
    subHook: "hbSubShip2",
  },
];
const a2 = (i: number) => A2[i];

// Format tags that "pour" into READ, and their inbound motion paths.
const FORMATS = [
  { t: "PPTX", path: "M58 280 C 78 300, 96 322, 110 344" },
  { t: ".docx", path: "M96 272 C 104 296, 110 320, 114 344" },
  { t: "JSON", path: "M140 278 C 132 304, 124 324, 118 344" },
  { t: "XLIFF", path: "M170 290 C 156 312, 138 330, 122 346" },
  { t: ".md", path: "M40 300 C 64 320, 90 334, 108 348" },
];

// Memory segments flying from the TM "sink" into RECYCLE.
const TM_SEGMENTS = [
  "M488 300 C 472 312, 458 322, 448 332",
  "M494 306 C 480 320, 464 330, 454 338",
  "M482 296 C 470 310, 454 318, 442 330",
  "M498 312 C 482 326, 464 334, 452 342",
];

// Language chips the TRANSLATE box shoots out — "fills every language".
const LANGS = [
  { t: "FR", path: "M446 477 C 400 470, 360 458, 326 452" },
  { t: "DE", path: "M446 477 C 396 488, 352 500, 312 506" },
  { t: "ES", path: "M446 477 C 410 506, 372 524, 338 532" },
  { t: "JA", path: "M446 477 C 392 462, 344 452, 300 448" },
  { t: "PT", path: "M446 477 C 404 500, 360 516, 320 522" },
];

export default function HeroStoryboard({ onOpen }: HeroStoryboardProps): React.ReactElement {
  const scope = useRef<HTMLDivElement>(null);

  useGSAP(
    () => {
      if (typeof window === "undefined") return;
      // Static, legible end-state under reduced motion — no timeline, no motion.
      if (window.matchMedia?.("(prefers-reduced-motion: reduce)").matches) return;

      gsap.registerPlugin(DrawSVGPlugin, MotionPathPlugin, useGSAP);
      const q = gsap.utils.selector(scope);

      const pop = { opacity: 0, scale: 0.55, transformOrigin: "50% 50%" };
      const popTo = (d = 0.34) => ({ opacity: 1, scale: 1, duration: d, ease: "back.out(1.8)" });
      const hide = { drawSVG: "0%" };
      const drawTo = (d: number) => ({ drawSVG: "100%", duration: d, ease: "power1.inOut" });

      const tl = gsap.timeline({ defaults: { ease: "power2.out" }, repeat: -1, repeatDelay: 2.6 });

      // Fly a set of particle nodes in along their per-element motion paths, then
      // fade them as they "arrive" (absorbed by the box).
      const flyIn = (nodes: Element[], paths: string[], at: string, each: number, dur: number) => {
        nodes.forEach((node, i) => {
          tl.fromTo(
            node,
            { opacity: 0 },
            {
              opacity: 1,
              duration: dur,
              ease: "power1.out",
              motionPath: { path: paths[i], alignOrigin: [0.5, 0.5] },
            },
            `${at}+=${i * each}`,
          );
        });
        tl.to(nodes, { opacity: 0, duration: 0.3, stagger: each }, `${at}+=${dur}`);
      };

      // Board frame draws itself.
      tl.fromTo(q(".hbFrame"), hide, drawTo(0.9));

      // ── Loop 1: the content loop ──────────────────────────────────────────
      tl.addLabel("a1", "-=0.15")
        .fromTo(
          q(".hbHl1"),
          { scaleX: 0, transformOrigin: "0% 50%" },
          { scaleX: 1, duration: 0.45, ease: "power3.inOut" },
          "a1",
        )
        .fromTo(
          q(".hbHead1"),
          { opacity: 0, y: 10 },
          { opacity: 1, y: 0, duration: 0.4 },
          "a1+=0.15",
        )
        .fromTo(q(".hbA1Box"), pop, { ...popTo(0.4), stagger: 0.13 }, "a1+=0.4")
        .fromTo(q(".hbA1Arrow"), hide, { ...drawTo(0.32), stagger: 0.13 }, "a1+=0.6")
        .fromTo(q(".hbA1Return"), hide, drawTo(0.55), ">-0.1")
        .fromTo(q(".hbReviseNote"), { opacity: 0 }, { opacity: 1, duration: 0.3 }, "<")
        .fromTo(
          q(".hbA1Sub"),
          { opacity: 0, y: 6 },
          { opacity: 1, y: 0, duration: 0.3, stagger: 0.07 },
          "<",
        );

      // ── Connector: SHIP → READ ─────────────────────────────────────────────
      tl.addLabel("conn", ">+0.05")
        .fromTo(q(".hbConn"), hide, drawTo(0.9), "conn")
        .fromTo(q(".hbConnNote"), { opacity: 0 }, { opacity: 1, duration: 0.4 }, "conn+=0.35");

      // ── Loop 2: going multilingual ─────────────────────────────────────────
      tl.addLabel("a2", ">-0.05")
        .fromTo(
          q(".hbHl2"),
          { scaleX: 0, transformOrigin: "0% 50%" },
          { scaleX: 1, duration: 0.45, ease: "power3.inOut" },
          "a2",
        )
        .fromTo(
          q(".hbHead2"),
          { opacity: 0, y: 10 },
          { opacity: 1, y: 0, duration: 0.4 },
          "a2+=0.15",
        );

      // READ + the format overflow pouring in.
      tl.fromTo(q(".hbBoxRead"), pop, popTo(0.36), ">").addLabel("fmt", "<");
      flyIn(
        q(".hbFmt"),
        FORMATS.map((c) => c.path),
        "fmt",
        0.1,
        0.55,
      );
      tl.fromTo(
        q(".hbSubRead"),
        { opacity: 0, y: 6 },
        { opacity: 1, y: 0, duration: 0.3 },
        "fmt+=0.5",
      );

      // READ → PREP
      tl.fromTo(q(".hbA2Arrow0"), hide, drawTo(0.3), "fmt+=0.55")
        .fromTo(q(".hbBoxPrep2"), pop, popTo(), ">-0.05")
        .fromTo(
          q(".hbSubPrep2"),
          { opacity: 0, y: 6 },
          { opacity: 1, y: 0, duration: 0.3 },
          "<+=0.1",
        );

      // PREP → RECYCLE + memory segments from the TM sink.
      tl.fromTo(q(".hbA2Arrow1"), hide, drawTo(0.3), ">-0.05")
        .fromTo(q(".hbBoxRecycle"), pop, popTo(), ">-0.05")
        .fromTo(
          q(".hbSink"),
          { opacity: 0, scale: 0.6, transformOrigin: "50% 50%" },
          { opacity: 1, scale: 1, duration: 0.3 },
          "<",
        )
        .addLabel("tm", "<+=0.1");
      flyIn(q(".hbTm"), TM_SEGMENTS, "tm", 0.1, 0.5);
      tl.fromTo(
        q(".hbSubRecycle"),
        { opacity: 0, y: 6 },
        { opacity: 1, y: 0, duration: 0.3 },
        "tm+=0.4",
      );

      // wrap RECYCLE → TRANSLATE + language chips shooting out.
      tl.fromTo(q(".hbWrap"), hide, drawTo(0.4), "tm+=0.55")
        .fromTo(q(".hbBoxTranslate"), pop, popTo(), ">-0.05")
        .addLabel("lang", "<+=0.05");
      flyIn(
        q(".hbLang"),
        LANGS.map((c) => c.path),
        "lang",
        0.09,
        0.65,
      );
      tl.fromTo(
        q(".hbSubTranslate"),
        { opacity: 0, y: 6 },
        { opacity: 1, y: 0, duration: 0.3 },
        "lang+=0.5",
      );

      // TRANSLATE → CHECK (red-line / green-line) → SHIP
      tl.fromTo(q(".hbA2Arrow2"), hide, drawTo(0.3), "lang+=0.6")
        .fromTo(q(".hbBoxCheck2"), pop, popTo(), ">-0.05")
        .fromTo(q(".hbTick"), hide, drawTo(0.35), "<+=0.1")
        .fromTo(q(".hbSubCheck2"), { opacity: 0, y: 6 }, { opacity: 1, y: 0, duration: 0.3 }, "<")
        .fromTo(q(".hbA2Arrow3"), hide, drawTo(0.3), ">-0.05")
        .fromTo(q(".hbBoxShip2"), pop, popTo(0.36), ">-0.05")
        .fromTo(
          q(".hbSubShip2"),
          { opacity: 0, y: 6 },
          { opacity: 1, y: 0, duration: 0.3 },
          "<+=0.1",
        );

      // The signature: the big return loop closes it, all the way back to READ.
      tl.addLabel("loop", ">+0.1")
        .fromTo(q(".hbRepeat"), hide, drawTo(1.05), "loop")
        .fromTo(q(".hbRepeatNote"), { opacity: 0 }, { opacity: 1, duration: 0.5 }, "loop+=0.45")
        .to(
          q(".hbBoxShip2"),
          { scale: 1.08, transformOrigin: "50% 50%", duration: 0.22, yoyo: true, repeat: 1 },
          ">-0.2",
        );
    },
    { scope },
  );

  return (
    <div className={s.root} ref={scope}>
      <button
        type="button"
        className={s.card}
        onClick={onOpen}
        aria-label="Open the interactive Try Kapi demo"
      >
        <span className={s.hint}>live demo →</span>
        <span className={s.srOnly}>
          A storyboard of kapi's two loops: the content loop — prep, write, check, ship — gets your
          English right; then going multilingual — read, prep, recycle, translate, check, ship —
          takes it into every language and writes every file back.
        </span>

        <svg
          className={s.board}
          viewBox="0 0 560 560"
          role="img"
          aria-hidden="true"
          xmlns="http://www.w3.org/2000/svg"
        >
          <defs>
            {/* Felt-tip wobble: a low-frequency displacement so marker strokes and
                box edges look hand-drawn, not vector-perfect. */}
            <filter id="hbRough" x="-6%" y="-6%" width="112%" height="112%">
              <feTurbulence
                type="fractalNoise"
                baseFrequency="0.016"
                numOctaves="2"
                seed="7"
                result="n"
              />
              <feDisplacementMap
                in="SourceGraphic"
                in2="n"
                scale="2"
                xChannelSelector="R"
                yChannelSelector="G"
              />
            </filter>
            <pattern id="hbGrid" width="26" height="26" patternUnits="userSpaceOnUse">
              <circle cx="1.4" cy="1.4" r="1.4" fill="var(--hb-grid)" />
            </pattern>
            <filter id="hbGrain">
              <feTurbulence
                type="fractalNoise"
                baseFrequency="0.9"
                numOctaves="2"
                stitchTiles="stitch"
                result="g"
              />
              <feColorMatrix in="g" type="saturate" values="0" />
            </filter>
            <clipPath id="hbClip">
              <rect x="14" y="14" width="532" height="532" rx="18" />
            </clipPath>
          </defs>

          {/* Paper + faint dot grid + grain. */}
          <rect
            className={`${s.frame} hbFrame`}
            x="14"
            y="14"
            width="532"
            height="532"
            rx="18"
            filter="url(#hbRough)"
          />
          <g clipPath="url(#hbClip)">
            <rect x="14" y="14" width="532" height="532" fill="url(#hbGrid)" />
            <rect x="14" y="14" width="532" height="532" filter="url(#hbGrain)" opacity="0.04" />
          </g>

          {/* ── Marker strokes (rough-filtered for the felt-tip edge) ── */}
          <g filter="url(#hbRough)">
            {A1.slice(0, 3).map((b, i) => (
              <path
                key={`a1arr-${i}`}
                className={`${s.stroke} hbA1Arrow`}
                d={arrow(b.cx + A1_W / 2 + 2, A1_CY, A1[i + 1].cx - A1_W / 2 - 2, A1_CY)}
              />
            ))}
            <path
              className={`${s.strokeThin} hbA1Return`}
              d={cubic(343.3, A1_TOP, 320, 82, 245, 82, 216.7, A1_TOP)}
            />

            {/* SHIP(loop 1) → READ(loop 2): drop down the right, run UNDER the
                GOING MULTILINGUAL header, then into READ — never crossing the head. */}
            <path
              className={`${s.stroke} hbConn`}
              d={`M 470 ${A1_TOP + A1_H} C 488 200, 482 270, 440 270 L 150 270 C 120 270, 114 298, 114 324 ${head(114, 300, 114, 330)}`}
            />

            <path
              className={`${s.stroke} hbA2Arrow0`}
              d={arrow(a2(0).cx + A2_W / 2 + 2, 355, a2(1).cx - A2_W / 2 - 2, 355)}
            />
            <path
              className={`${s.stroke} hbA2Arrow1`}
              d={arrow(a2(1).cx + A2_W / 2 + 2, 355, a2(2).cx - A2_W / 2 - 2, 355)}
            />
            <path
              className={`${s.stroke} hbA2Arrow2`}
              d={arrow(a2(3).cx - A2_W / 2 - 2, 477, a2(4).cx + A2_W / 2 + 2, 477)}
            />
            <path
              className={`${s.stroke} hbA2Arrow3`}
              d={arrow(a2(4).cx - A2_W / 2 - 2, 477, a2(5).cx + A2_W / 2 + 2, 477)}
            />
            <path
              className={`${s.stroke} hbWrap`}
              d={cubic(446, 355 + A2_H / 2, 520, 392, 520, 440, 450, 477 - A2_H / 2)}
            />

            <path
              className={`${s.loop} hbRepeat`}
              d={`M ${a2(5).cx} ${477 + A2_H / 2} C 64 540, 22 520, 22 470 L 22 392 C 22 360, 28 348, 40 355 ${head(30, 349, 40, 355)}`}
            />

            <g className="hbSink">
              <path className={s.strokeThin} d="M474 286 L506 286 L494 304 L486 304 Z" />
            </g>
            <path className={`${s.loop} hbTick`} d="M262 478 l7 8 l13 -16" />
          </g>

          {/* ── Type + boxes (crisp; box rects rough-filtered individually) ── */}
          {/* Loop 1 header */}
          <g transform="rotate(-1.2 156 55)">
            <rect className={`${s.highlight} hbHl1`} x="40" y="42" width="232" height="26" rx="4" />
          </g>
          <g className="hbHead1">
            <text className={`${s.marker} ${s.headText}`} x="48" y="63" fontSize="21">
              THE CONTENT LOOP
            </text>
            <text className={`${s.hand} ${s.note}`} x="48" y="88" fontSize="15.5">
              get the English right — write ⇄ check, then ship
            </text>
          </g>
          <text
            className={`${s.hand} ${s.note} hbReviseNote`}
            x="280"
            y="78"
            fontSize="14.5"
            textAnchor="middle"
          >
            revise ↺
          </text>

          {/* Loop 1 boxes (rect + label pop together) */}
          {A1.map((b) => (
            <g key={`a1box-${b.id}`} className="hbA1Box">
              <rect
                className={b.accent ? s.boxFillGreen : s.boxFill}
                x={f(b.cx - A1_W / 2)}
                y={A1_TOP}
                width={A1_W}
                height={A1_H}
                rx="9"
                filter="url(#hbRough)"
              />
              <text
                className={`${s.marker} ${s.boxLabel}`}
                x={b.cx}
                y={A1_CY}
                fontSize="17"
                textAnchor="middle"
                dominantBaseline="central"
              >
                {b.label}
              </text>
            </g>
          ))}
          {A1.map((b) => (
            <text
              key={`a1sub-${b.id}`}
              className={`${s.hand} ${s.boxSub} hbA1Sub`}
              x={b.cx}
              y={A1_TOP + A1_H + 16}
              fontSize="13.5"
              textAnchor="middle"
            >
              {b.sub}
            </text>
          ))}

          {/* Connector note */}
          <text
            className={`${s.hand} ${s.note} hbConnNote`}
            x="300"
            y="201"
            fontSize="14.5"
            textAnchor="middle"
          >
            shipped → flows back in, every language
          </text>

          {/* Loop 2 header */}
          <g transform="rotate(-1.1 184 241)">
            <rect
              className={`${s.highlight} hbHl2`}
              x="40"
              y="228"
              width="288"
              height="26"
              rx="4"
            />
          </g>
          <text className={`${s.marker} ${s.headText} hbHead2`} x="48" y="249" fontSize="21">
            GOING MULTILINGUAL
          </text>

          {/* Loop 2 boxes */}
          {A2.map((b) => (
            <g key={`a2box-${b.id}`} className={b.hook}>
              <rect
                className={b.accent || b.green ? s.boxFillGreen : s.boxFill}
                x={f(b.cx - A2_W / 2)}
                y={f(b.cy - A2_H / 2)}
                width={A2_W}
                height={A2_H}
                rx="10"
                filter="url(#hbRough)"
              />
              <text
                className={`${s.marker} ${s.boxLabel}`}
                x={b.cx}
                y={b.cy}
                fontSize={b.label.length > 6 ? 15.5 : 16.5}
                textAnchor="middle"
                dominantBaseline="central"
              >
                {b.label}
              </text>
            </g>
          ))}
          {A2.map((b) => (
            <text
              key={`a2sub-${b.id}`}
              className={`${s.hand} ${s.boxSub} ${b.subHook}`}
              x={b.cx}
              y={b.cy + A2_H / 2 + 16}
              fontSize="13"
              textAnchor="middle"
            >
              {b.sub}
            </text>
          ))}

          {/* TM sink label */}
          <text
            className={`${s.hand} ${s.note} hbSink`}
            x="490"
            y="280"
            fontSize="12.5"
            textAnchor="middle"
          >
            TM
          </text>

          {/* Big-loop note, up the left margin (kept clear of the frame edge) */}
          <text
            className={`${s.hand} ${s.note} hbRepeatNote`}
            x="33"
            y="416"
            fontSize="15"
            textAnchor="middle"
            transform="rotate(-90 33 416)"
          >
            REPEAT ↻ every change
          </text>

          {/* ── Particles (hidden until the timeline drives them along paths) ── */}
          {FORMATS.map((c, i) => (
            <g key={`fmt-${i}`} className={`${s.particle} hbFmt`}>
              <rect className={s.chip} x="-22" y="-9" width="44" height="18" rx="4" />
              <text
                className={`${s.hand} ${s.chipInk}`}
                x="0"
                y="0"
                textAnchor="middle"
                dominantBaseline="central"
              >
                {c.t}
              </text>
            </g>
          ))}
          {TM_SEGMENTS.map((_, i) => (
            <rect
              key={`tm-${i}`}
              className={`${s.particle} ${s.seg} hbTm`}
              x="-11"
              y="-4"
              width="22"
              height="8"
              rx="2"
            />
          ))}
          {LANGS.map((c, i) => (
            <g key={`lang-${i}`} className={`${s.particle} hbLang`}>
              <rect className={s.boxFillGreen} x="-16" y="-10" width="32" height="20" rx="5" />
              <text
                className={`${s.marker} ${s.chipInk}`}
                x="0"
                y="0.5"
                textAnchor="middle"
                dominantBaseline="central"
                fontSize="11"
              >
                {c.t}
              </text>
            </g>
          ))}
        </svg>
      </button>

      <p className={`${s.hand} ${s.caption}`}>
        Two loops: <b>get it right</b>, then <b>get it everywhere</b>.
      </p>

      <button type="button" className={s.cta} onClick={onOpen}>
        Try Kapi in your browser <ArrowRight size={16} aria-hidden="true" />
      </button>
    </div>
  );
}
