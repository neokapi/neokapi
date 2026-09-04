/**
 * The bridge: one brand palette in, one set of Docusaurus and diagram-kit
 * custom properties out.
 *
 * Docusaurus paints from Infima's `--ifm-*` variables and the diagram kit from
 * its own `--kdx-*` ones, and neither speaks OKLCH. Rather than keep a hand-kept
 * hex ramp beside each brand palette, every value below is computed from the
 * canonical tokens: the surfaces come straight across, the primary ramp is seven
 * lightness steps of the brand primary, and each diagram role takes its hue from
 * the shared semantic tokens so a role means the same thing on both sites.
 *
 * Two rules hold the result legible:
 *
 *   - Text colours are pushed along their own lightness axis until they clear
 *     WCAG AA (4.5:1) against the ground they sit on. A brand primary picked for
 *     a button on a card is usually too light to be a link on a page, and the
 *     search is what makes the docs primary a shade of the brand rather than a
 *     second opinion about it.
 *   - Diagram accents sit at one lightness per theme, so six roles drawn side by
 *     side read as one drawing.
 */

import {
  type Oklch,
  accent,
  contrastRatio,
  forContrast,
  luminance,
  mixLightness,
  parseOklch,
  toHex,
  toRgba,
  withLightness,
} from "./oklch.ts";
import type { CanonicalPalette, TokenMap } from "./tokens.ts";

/** WCAG AA for body text. Every text colour the bridge emits clears it. */
export const AA_TEXT = 4.5;

/** Named steps of the Infima primary ramp, as multipliers of the base lightness. */
const RAMP: ReadonlyArray<readonly [string, number]> = [
  ["darkest", 0.72],
  ["darker", 0.86],
  ["dark", 0.92],
  ["", 1],
  ["light", 1.08],
  ["lighter", 1.14],
  ["lightest", 1.28],
];

/** Where each diagram role takes its hue. `io` is the brand itself. */
const ROLE_HUES: ReadonlyArray<readonly [string, string]> = [
  ["annotate", "axis-brand"],
  ["check", "warning"],
  ["plugin", "axis-product"],
  ["resource", "success"],
  ["translate", "axis-language"],
];

/** One lightness and chroma for every diagram accent, per theme. */
const ACCENT = {
  light: { l: 0.5, c: 0.15 },
  dark: { l: 0.78, c: 0.13 },
} as const;

export type ThemeName = "light" | "dark";

/** A rendered custom property: the name without `--`, and its printed value. */
export interface Declaration {
  name: string;
  value: string;
}

export interface ThemeBridge {
  /** `--ifm-*` and `--docusaurus-*` properties, for `:root` / `[data-theme="dark"]`. */
  site: Declaration[];
  /** `--kdx-*` properties, for the diagram kit's scope. */
  diagram: Declaration[];
}

export interface Bridge {
  light: ThemeBridge;
  dark: ThemeBridge;
}

class MissingToken extends Error {
  constructor(theme: ThemeName, token: string) {
    super(`the ${theme} palette declares no --${token}`);
    this.name = "MissingToken";
  }
}

function color(tokens: TokenMap, theme: ThemeName, name: string): Oklch {
  const raw = tokens.get(name);
  if (raw === undefined) throw new MissingToken(theme, name);
  const parsed = parseOklch(raw);
  if (parsed === null) {
    throw new Error(`--${name} is "${raw}", which is not an oklch() colour`);
  }
  return parsed;
}

function text(tokens: TokenMap, theme: ThemeName, name: string): string | undefined {
  return tokens.get(name);
}

/**
 * The ground a text colour has the least room against. A light theme puts dark
 * ink on every ground, so the darkest one is the tightest; a dark theme puts
 * light ink on them, so the lightest one is.
 */
function hardestGround(grounds: Oklch[]): Oklch {
  const lightTheme = luminance(grounds[0]) > 0.18;
  return grounds.reduce((worst, ground) => {
    const closer = lightTheme
      ? luminance(ground) < luminance(worst)
      : luminance(ground) > luminance(worst);
    return closer ? ground : worst;
  });
}

function themeBridge(tokens: TokenMap, theme: ThemeName): ThemeBridge {
  const background = color(tokens, theme, "background");
  const surface = color(tokens, theme, "card");
  const foreground = color(tokens, theme, "foreground");
  const muted = color(tokens, theme, "muted");
  const mutedForeground = color(tokens, theme, "muted-foreground");
  const border = color(tokens, theme, "border");
  const brand = color(tokens, theme, "primary");

  // A docs page draws text on three grounds: the page, a card, and the band
  // behind inline code. Fixing a text colour against the page alone leaves it
  // short on one of the other two, so each one is fixed against the hardest of
  // the three and clears AA everywhere it can land.
  const ground = hardestGround([background, surface, muted]);
  const primary = forContrast(brand, ground, AA_TEXT);
  const ink = forContrast(foreground, ground, AA_TEXT);
  const quiet = forContrast(mutedForeground, ground, AA_TEXT);

  const site: Declaration[] = [];
  for (const [step, factor] of RAMP) {
    site.push({
      name: step === "" ? "ifm-color-primary" : `ifm-color-primary-${step}`,
      value: toHex(withLightness(primary, primary.l * factor)),
    });
  }
  site.push(
    { name: "ifm-background-color", value: toHex(background) },
    { name: "ifm-background-surface-color", value: toHex(surface) },
    { name: "ifm-font-color-base", value: toHex(ink) },
    { name: "ifm-heading-color", value: toHex(ink) },
    { name: "ifm-code-background", value: toHex(muted) },
    { name: "ifm-toc-border-color", value: toHex(border) },
    { name: "ifm-hr-border-color", value: toHex(border) },
    {
      name: "docusaurus-highlighted-code-line-bg",
      value: toRgba(ink, theme === "light" ? 0.08 : 0.14),
    },
  );

  const sans = text(tokens, theme, "brand-font-sans");
  if (sans !== undefined) site.push({ name: "ifm-font-family-base", value: sans });
  const mono = text(tokens, theme, "brand-font-mono");
  if (mono !== undefined) site.push({ name: "ifm-font-family-monospace", value: mono });

  const { l, c } = ACCENT[theme];
  const io = accent(brand, l, c);
  const diagram: Declaration[] = [
    { name: "kdx-border", value: toHex(border) },
    // A channel line is quieter than the ink beside it and firmer than a border.
    { name: "kdx-channel", value: toHex(mixLightness(border, mutedForeground, 0.4)) },
    { name: "kdx-file", value: toHex(io) },
    { name: "kdx-ink", value: toHex(ink) },
    { name: "kdx-io", value: toHex(io) },
    { name: "kdx-muted", value: toHex(quiet) },
    { name: "kdx-surface", value: toHex(surface) },
    { name: "kdx-surface-2", value: toHex(muted) },
  ];
  for (const [role, hue] of ROLE_HUES) {
    diagram.push({ name: `kdx-${role}`, value: toHex(accent(color(tokens, theme, hue), l, c)) });
  }
  if (mono !== undefined) diagram.push({ name: "kdx-mono", value: mono });
  diagram.sort((a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1 : 0));

  return { site, diagram };
}

/** The whole bridge for one canonical palette. */
export function deriveBridge(palette: CanonicalPalette): Bridge {
  return {
    light: themeBridge(palette.light, "light"),
    dark: themeBridge(palette.dark, "dark"),
  };
}

/**
 * The contrast pairs the bridge is answerable for: what a reader reads, against
 * what it is read on. Tests assert every one of them clears AA on the real
 * palettes, so a palette edit that pushes a link under the bar fails there.
 */
export function textContrasts(palette: CanonicalPalette): Array<{
  theme: ThemeName;
  what: string;
  ratio: number;
}> {
  const out: Array<{ theme: ThemeName; what: string; ratio: number }> = [];
  for (const theme of ["light", "dark"] as const) {
    const tokens = theme === "light" ? palette.light : palette.dark;
    const background = color(tokens, theme, "background");
    const surface = color(tokens, theme, "card");
    const band = color(tokens, theme, "muted");
    const ground = hardestGround([background, surface, band]);
    const primary = forContrast(color(tokens, theme, "primary"), ground, AA_TEXT);
    const ink = forContrast(color(tokens, theme, "foreground"), ground, AA_TEXT);
    const quiet = forContrast(color(tokens, theme, "muted-foreground"), ground, AA_TEXT);
    out.push(
      { theme, what: "link on the page", ratio: contrastRatio(primary, background) },
      { theme, what: "link on a surface", ratio: contrastRatio(primary, surface) },
      { theme, what: "link on a code band", ratio: contrastRatio(primary, band) },
      { theme, what: "body text on the page", ratio: contrastRatio(ink, background) },
      { theme, what: "body text on a surface", ratio: contrastRatio(ink, surface) },
      { theme, what: "body text on a code band", ratio: contrastRatio(ink, band) },
      { theme, what: "muted text on the page", ratio: contrastRatio(quiet, background) },
      { theme, what: "muted text on a code band", ratio: contrastRatio(quiet, band) },
    );

    const { l, c } = ACCENT[theme];
    const diagramSurface = surface;
    const diagramSurface2 = band;
    const roles: Array<[string, Oklch]> = [["io", accent(color(tokens, theme, "primary"), l, c)]];
    for (const [role, hue] of ROLE_HUES)
      roles.push([role, accent(color(tokens, theme, hue), l, c)]);
    for (const [role, value] of roles) {
      out.push(
        { theme, what: `diagram ${role} on a card`, ratio: contrastRatio(value, diagramSurface) },
        {
          theme,
          what: `diagram ${role} on a band`,
          ratio: contrastRatio(value, diagramSurface2),
        },
      );
    }
  }
  return out;
}
