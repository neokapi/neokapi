/**
 * Reading the canonical palettes off disk and writing the bridges back.
 *
 * `generate` rewrites every target; `check` reports which ones no longer match
 * their source, which is the drift gate CI runs.
 */

import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";

import { deriveBridge } from "./derive.ts";
import { renderBridge } from "./render.ts";
import { MAKE_TARGET, TARGETS, type Target } from "./targets.ts";
import { type CanonicalPalette, importedFiles, mergePalettes, readPalette } from "./tokens.ts";

/**
 * One palette file plus everything it imports, merged in cascade order: the
 * shared semantic tokens arrive first and the brand's own values may override
 * them. Only relative specifiers are followed, so a package import stays out.
 */
export function loadCanonical(path: string): CanonicalPalette {
  const css = readFileSync(path, "utf8");
  let palette: CanonicalPalette = { light: new Map(), dark: new Map() };
  for (const specifier of importedFiles(css)) {
    if (!specifier.startsWith(".")) continue;
    palette = mergePalettes(palette, loadCanonical(resolve(dirname(path), specifier)));
  }
  return mergePalettes(palette, readPalette(css));
}

/** The file one target should hold, given the palette it is computed from. */
export function renderTarget(repoRoot: string, target: Target): string {
  const palette = loadCanonical(join(repoRoot, target.source));
  return renderBridge(deriveBridge(palette), { ...target, target: MAKE_TARGET });
}

export interface TargetResult {
  out: string;
  /** True when the file on disk already held exactly what was rendered. */
  current: boolean;
}

function readIfPresent(path: string): string | null {
  try {
    return readFileSync(path, "utf8");
  } catch {
    return null;
  }
}

/** Rewrite every target. Returns one result per file. */
export function generate(repoRoot: string, targets: readonly Target[] = TARGETS): TargetResult[] {
  return targets.map((target) => {
    const rendered = renderTarget(repoRoot, target);
    const path = join(repoRoot, target.out);
    const current = readIfPresent(path) === rendered;
    if (!current) writeFileSync(path, rendered);
    return { out: target.out, current };
  });
}

/** Report every target whose committed file is stale, writing nothing. */
export function check(repoRoot: string, targets: readonly Target[] = TARGETS): TargetResult[] {
  return targets.map((target) => ({
    out: target.out,
    current: readIfPresent(join(repoRoot, target.out)) === renderTarget(repoRoot, target),
  }));
}
