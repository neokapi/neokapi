/**
 * The drawing kit an explainer's beats share: one 1600x900 SVG stage scaled to
 * the 1920x1080 frame, the type scale, and the calm motion helpers.
 *
 * Every beat is authored in the storyboard's own coordinates (a 1600x900
 * viewBox), so a beat's settled frame is the approved still drawn at 1.2x.
 */
import React from "react";
import { Easing, interpolate } from "remotion";
import type { TimedCaption } from "../../types.ts";
import { theme } from "../components/theme.ts";

export const STAGE_W = 1600;
export const STAGE_H = 900;

/** What every beat receives: its narration's word timings and its length. */
export interface BeatProps {
  /** Frame at which a spoken word starts (see cueFrames); the fallback when the word is not found. */
  cue: (word: string, fallbackFrame: number, nth?: number) => number;
  /** Frames the beat is on screen, transitions included. */
  durationFrames: number;
}

/** A beat's cue lookup over its scene's captions: the nth word whose letters match. */
export function cueFrames(captions: TimedCaption[], fps: number): BeatProps["cue"] {
  const norm = (t: string) => t.toLowerCase().replace(/[^\p{L}\p{N}]+/gu, "");
  return (word, fallbackFrame, nth = 1) => {
    const want = norm(word);
    let seen = 0;
    for (const c of captions) {
      if (norm(c.text) !== want) continue;
      seen++;
      if (seen === nth) return Math.round((c.startMs / 1000) * fps);
    }
    return fallbackFrame;
  };
}

/** 0 to 1 over [start, start + dur], eased in and out: the one curve every motion uses. */
export function ease(frame: number, start: number, dur: number): number {
  return interpolate(frame, [start, start + dur], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
    easing: Easing.inOut(Easing.cubic),
  });
}

export function lerp(a: number, b: number, p: number): number {
  return a + (b - a) * p;
}

/** The stage: the storyboard's viewBox filling the frame on the frame colour. */
export const Stage: React.FC<{ children: React.ReactNode }> = ({ children }) => (
  <svg
    viewBox={`0 0 ${STAGE_W} ${STAGE_H}`}
    width="100%"
    height="100%"
    style={{ position: "absolute", inset: 0, background: theme.panel }}
    fontFamily={theme.fontSans}
  >
    {children}
  </svg>
);

/** The storyboard's type scale, as SVG text props. */
export const type = {
  xl: () => ({ fontSize: 84, fontWeight: 700, fill: theme.text, letterSpacing: -2 }),
  l: () => ({ fontSize: 64, fontWeight: 700, fill: theme.text, letterSpacing: -1 }),
  m: () => ({ fontSize: 30, fontWeight: 600, fill: theme.text }),
  s: () => ({ fontSize: 22, fontWeight: 500, fill: theme.dim }),
  xs: () => ({ fontSize: 17, fontWeight: 600, fill: theme.dim, letterSpacing: 1.5 }),
  body: () => ({ fontSize: 25, fontWeight: 400, fill: theme.text }),
  mono: () => ({ fontSize: 23, fontWeight: 400, fill: theme.text, fontFamily: theme.fontMono }),
};

/** A small-caps label: the storyboard's uppercase eyebrow. */
export const Eyebrow: React.FC<{ x: number; y: number; text: string; anchor?: "start" | "middle" | "end"; opacity?: number; size?: number }> = ({
  x,
  y,
  text,
  anchor = "start",
  opacity = 1,
  size,
}) => (
  <text x={x} y={y} textAnchor={anchor} opacity={opacity} {...type.xs()} {...(size ? { fontSize: size } : {})}>
    {text.toUpperCase()}
  </text>
);

/** Fade a group in while it rises a few units into place. */
export const Rise: React.FC<{ p: number; dy?: number; children: React.ReactNode }> = ({ p, dy = 14, children }) => (
  <g opacity={p} transform={`translate(0 ${(1 - p) * dy})`}>
    {children}
  </g>
);

/** A rule: the accent-tinted pill or bar every rule is drawn as. */
export function ruleFill() {
  return { fill: theme.accentSoft, stroke: theme.accent, strokeWidth: 2 };
}

/** A card's outline at rest. */
export function cardFill() {
  return { fill: theme.panel, stroke: theme.panelBorder, strokeWidth: 2 };
}

/**
 * A line drawn from its start: a path or line whose dash reveals `p` of its
 * length. `length` is the path's length in stage units.
 */
export function drawn(p: number, length: number) {
  return { strokeDasharray: length, strokeDashoffset: length * (1 - p) };
}
