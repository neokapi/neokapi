/**
 * OKLCH arithmetic and sRGB output, with no dependencies.
 *
 * The canonical brand palettes are written in OKLCH because a lightness step
 * there is a step a reader sees. Docusaurus wants sRGB hex, so the bridge
 * converts. Everything here is pure and deterministic: the same OKLCH triple
 * always renders the same hex, which is what lets the committed bridge files
 * carry a byte gate.
 *
 * Contrast is measured on the ROUNDED 8-bit channels, so a ratio asserted in a
 * test is the ratio a browser computes from the emitted hex rather than one
 * from a higher-precision intermediate.
 */

export interface Oklch {
  /** Perceptual lightness, 0 (black) to 1 (white). */
  l: number;
  /** Chroma. 0 is neutral gray; the sRGB gamut runs out around 0.37. */
  c: number;
  /** Hue angle in degrees. */
  h: number;
}

const OKLCH_PATTERN =
  /^oklch\(\s*([0-9.]+)(%?)\s+([0-9.]+)(%?)\s+([0-9.]+)(?:deg)?\s*(?:\/\s*[0-9.]+%?\s*)?\)$/i;

/** Parse an `oklch(L C H)` (or `oklch(L C H / A)`) value. Alpha is discarded. */
export function parseOklch(value: string): Oklch | null {
  const match = OKLCH_PATTERN.exec(value.trim());
  if (!match) return null;
  const l = Number(match[1]) / (match[2] === "%" ? 100 : 1);
  const c = Number(match[3]) / (match[4] === "%" ? 250 : 1);
  const h = Number(match[5]);
  if (!Number.isFinite(l) || !Number.isFinite(c) || !Number.isFinite(h)) return null;
  return { l, c, h };
}

/** The same colour at a different lightness, clamped to a printable range. */
export function withLightness(color: Oklch, l: number): Oklch {
  return { ...color, l: clamp(l, 0.02, 0.99) };
}

/** A colour built from one token's hue and an explicit lightness and chroma. */
export function accent(hueSource: Oklch, l: number, c: number): Oklch {
  return { l: clamp(l, 0.02, 0.99), c, h: hueSource.h };
}

/** The midpoint of two colours in lightness, keeping the first one's hue. */
export function mixLightness(base: Oklch, other: Oklch, t: number): Oklch {
  return withLightness(base, base.l + (other.l - base.l) * t);
}

function clamp(v: number, lo: number, hi: number): number {
  return Math.min(hi, Math.max(lo, v));
}

function linearSrgb(color: Oklch): [number, number, number] {
  const rad = (color.h * Math.PI) / 180;
  const a = color.c * Math.cos(rad);
  const b = color.c * Math.sin(rad);

  const lp = color.l + 0.3963377774 * a + 0.2158037573 * b;
  const mp = color.l - 0.1055613458 * a - 0.0638541728 * b;
  const sp = color.l - 0.0894841775 * a - 1.291485548 * b;

  const l3 = lp * lp * lp;
  const m3 = mp * mp * mp;
  const s3 = sp * sp * sp;

  return [
    4.0767416621 * l3 - 3.3077115913 * m3 + 0.2309699292 * s3,
    -1.2684380046 * l3 + 2.6097574011 * m3 - 0.3413193965 * s3,
    -0.0041960863 * l3 - 0.7034186147 * m3 + 1.707614701 * s3,
  ];
}

const GAMUT_EPSILON = 1e-4;

function inGamut(rgb: [number, number, number]): boolean {
  return rgb.every((v) => v >= -GAMUT_EPSILON && v <= 1 + GAMUT_EPSILON);
}

/**
 * The nearest in-gamut colour at the same lightness and hue, found by halving
 * the chroma. Twenty bisection steps land well inside a rounding step of an
 * 8-bit channel, so the result is stable.
 */
export function toGamut(color: Oklch): Oklch {
  if (inGamut(linearSrgb(color))) return color;
  let lo = 0;
  let hi = color.c;
  for (let i = 0; i < 20; i++) {
    const mid = (lo + hi) / 2;
    if (inGamut(linearSrgb({ ...color, c: mid }))) lo = mid;
    else hi = mid;
  }
  return { ...color, c: lo };
}

function encode(channel: number): number {
  const v = clamp(channel, 0, 1);
  const encoded = v <= 0.0031308 ? 12.92 * v : 1.055 * Math.pow(v, 1 / 2.4) - 0.055;
  return Math.round(clamp(encoded, 0, 1) * 255);
}

/** The colour's 8-bit sRGB channels, gamut-mapped. */
export function toRgb(color: Oklch): [number, number, number] {
  const rgb = linearSrgb(toGamut(color));
  return [encode(rgb[0]), encode(rgb[1]), encode(rgb[2])];
}

/** `#rrggbb`, lowercase. */
export function toHex(color: Oklch): string {
  return `#${toRgb(color)
    .map((v) => v.toString(16).padStart(2, "0"))
    .join("")}`;
}

/** `rgba(r, g, b, a)` with the alpha printed to two decimals. */
export function toRgba(color: Oklch, alpha: number): string {
  const [r, g, b] = toRgb(color);
  return `rgba(${r}, ${g}, ${b}, ${alpha.toFixed(2)})`;
}

/** WCAG relative luminance of an `#rrggbb` value. */
export function hexLuminance(hex: string): number {
  const channels = [1, 3, 5].map((at) => {
    const s = parseInt(hex.slice(at, at + 2), 16) / 255;
    return s <= 0.04045 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4);
  });
  return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2];
}

/** WCAG contrast ratio between two `#rrggbb` values, 1 to 21. */
export function hexContrast(a: string, b: string): number {
  const la = hexLuminance(a);
  const lb = hexLuminance(b);
  const [hi, lo] = la >= lb ? [la, lb] : [lb, la];
  return (hi + 0.05) / (lo + 0.05);
}

/** WCAG relative luminance of the colour as it will be written out. */
export function luminance(color: Oklch): number {
  return hexLuminance(toHex(color));
}

/** WCAG contrast ratio between two colours, as they will be written out. */
export function contrastRatio(a: Oklch, b: Oklch): number {
  return hexContrast(toHex(a), toHex(b));
}

/**
 * The colour moved along its own lightness axis, in fixed steps, until it
 * clears `target` contrast against `against`. Light grounds darken it and dark
 * grounds lighten it, so one rule covers both themes. The step is coarse enough
 * that the answer reads as a chosen value rather than as a search artefact.
 */
export function forContrast(color: Oklch, against: Oklch, target: number): Oklch {
  const towardsDark = luminance(against) > 0.18;
  const step = towardsDark ? -0.005 : 0.005;
  let candidate = color;
  for (let i = 0; i < 200; i++) {
    if (contrastRatio(candidate, against) >= target) return candidate;
    const next = withLightness(candidate, candidate.l + step);
    if (next.l === candidate.l) break;
    candidate = next;
  }
  return candidate;
}
