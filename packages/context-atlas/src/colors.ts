// What colour says in the atlas. Categories take eight fixed hues, assigned
// by a value's place in its axis catalogue so a filter never repaints the
// survivors; a ninth value onwards takes the neutral. States take the shared
// judgement colours. The lenses use one sequential and one diverging scale
// built from those same tokens.
//
// The categorical steps were checked with the dataviz palette validator
// (lightness band, chroma floor, adjacent colour-vision separation, normal
// vision floor, 3:1 against the surface) on white and on the dark card.

import { type Atlas, NOT_SET } from "./model/atlas";
import { type StateTone, stateTone } from "./model/state";

/** The palette's custom properties, light and dark, scoped to the atlas root. */
export const ATLAS_STYLE = `
.context-atlas {
  --atlas-cat-1: oklch(0.58 0.13 195);
  --atlas-cat-2: oklch(0.6 0.19 25);
  --atlas-cat-3: oklch(0.52 0.15 255);
  --atlas-cat-4: oklch(0.665 0.145 72);
  --atlas-cat-5: oklch(0.55 0.17 300);
  --atlas-cat-6: oklch(0.6 0.15 145);
  --atlas-cat-7: oklch(0.56 0.18 345);
  --atlas-cat-8: oklch(0.64 0.14 110);
  --atlas-neutral: color-mix(in oklab, var(--muted-foreground) 70%, var(--background));
  --atlas-ribbon: color-mix(in oklab, var(--muted-foreground) 22%, var(--background));
  --atlas-land: color-mix(in oklab, var(--muted-foreground) 13%, var(--background));
  --atlas-border: color-mix(in oklab, var(--muted-foreground) 45%, var(--background));
  --atlas-seq: var(--primary);
}
.dark .context-atlas {
  --atlas-cat-1: oklch(0.64 0.12 195);
  --atlas-cat-2: oklch(0.62 0.17 25);
  --atlas-cat-3: oklch(0.6 0.14 255);
  --atlas-cat-4: oklch(0.665 0.14 75);
  --atlas-cat-5: oklch(0.63 0.15 300);
  --atlas-cat-6: oklch(0.64 0.15 145);
  --atlas-cat-7: oklch(0.62 0.17 345);
  --atlas-cat-8: oklch(0.66 0.13 110);
  --atlas-neutral: color-mix(in oklab, var(--muted-foreground) 60%, var(--background));
  --atlas-ribbon: color-mix(in oklab, var(--muted-foreground) 26%, var(--background));
  --atlas-land: color-mix(in oklab, var(--muted-foreground) 20%, var(--background));
  --atlas-border: color-mix(in oklab, var(--muted-foreground) 50%, var(--background));
  --atlas-seq: var(--primary);
}
@media (prefers-reduced-motion: reduce) {
  .context-atlas * { transition: none !important; animation: none !important; }
}
`;

export const CATEGORY_SLOTS = 8;

/** The neutral a mark takes when it carries no single category. */
export const NEUTRAL = "var(--atlas-neutral)";

/** The faint ground of the surrounding context's ribbons. */
export const RIBBON_GROUND = "var(--atlas-ribbon)";

const STATE_FILL: Record<StateTone, string> = {
  settled: "var(--success)",
  pending: "var(--info)",
  attention: "var(--warning)",
  middle: "color-mix(in oklab, var(--foreground) 40%, var(--background))",
  start: "color-mix(in oklab, var(--muted-foreground) 45%, var(--background))",
  closed: "color-mix(in oklab, var(--muted-foreground) 30%, var(--background))",
  unknown: "color-mix(in oklab, var(--muted-foreground) 30%, var(--background))",
  absent: "transparent",
};

/** Land on the map where nothing sits. */
export const LAND = "var(--atlas-land)";

/** The line between two shapes on the map. */
export const BORDER = "var(--atlas-border)";

/**
 * Sequential, for a map: land at 0, the primary hue at 1. A share above 0
 * starts visibly off the land so the smallest count still reads.
 */
export function sequentialFill(share: number | null): string {
  if (share === null || !(share > 0)) return LAND;
  const pct = Math.round(18 + Math.min(1, share) * 72);
  return `color-mix(in oklab, var(--atlas-seq) ${pct}%, var(--atlas-land))`;
}

/** The stripe of the hatch drawn where a record at a wider place holds. */
export const HATCH_STRIPE = "color-mix(in oklab, var(--atlas-seq) 60%, var(--atlas-land))";

/** The fill for a state. */
export function stateFill(state: string): string {
  return STATE_FILL[stateTone(state)];
}

/** The fill for a value of an axis: its state colour on a status axis, else its category hue. */
export function valueFill(atlas: Atlas, axis: string, value: string): string {
  const info = atlas.axisById.get(axis);
  if (info?.origin === "view" && (axis === "status-start" || axis === "status-snapshot")) {
    return stateFill(value);
  }
  if (value === NOT_SET || !info) return NEUTRAL;
  const index = info.values.findIndex((v) => v.id === value);
  if (index < 0 || index >= CATEGORY_SLOTS) return NEUTRAL;
  return `var(--atlas-cat-${index + 1})`;
}

/** Sequential: neutral at 0, the warning amber at 1. */
export function attentionFill(share: number | null): string {
  if (share === null) return NEUTRAL;
  const pct = Math.round(Math.min(1, Math.max(0, share)) * 100);
  return `color-mix(in oklab, var(--warning) ${pct}%, var(--atlas-neutral))`;
}

/**
 * The same sequential scale for a cell that carries text: from the muted
 * ground at 0 to the warning amber at 1, so the ink stays legible at the low
 * end. Past half, write on it in `--warning-foreground`.
 */
export function attentionTint(share: number | null): string {
  if (share === null) return "transparent";
  const pct = Math.round(Math.min(1, Math.max(0, share)) * 100);
  return `color-mix(in oklab, var(--warning) ${pct}%, var(--muted))`;
}

/**
 * Diverging: the info blue for a falling share, neutral for none, the warning
 * amber for a rising one. Saturates at 50 percentage points; the labels carry
 * the exact value.
 */
export function changeFill(delta: number | null): string {
  if (delta === null) return NEUTRAL;
  const mag = Math.round(Math.min(1, Math.abs(delta) / 0.5) * 100);
  const pole = delta >= 0 ? "var(--warning)" : "var(--info)";
  return `color-mix(in oklab, ${pole} ${mag}%, var(--atlas-neutral))`;
}
