/**
 * Format engines, as the formats listing reports them: `native` for the
 * built-in formats, else a plugin's name. A file whose extension more than
 * one engine claims is read by whichever engine the project's order chooses;
 * these helpers let a surface name the alternatives and pick one engine's
 * format outright, the way `html@okapi-bridge` does on the CLI.
 */

import type { FormatInfo } from "../types/api";

export const NATIVE_ENGINE = "native";

/** The engine a format belongs to: `engine` as listed, else derived from `source`. */
export function engineOf(f: Pick<FormatInfo, "engine" | "source">): string {
  if (f.engine) return f.engine;
  if (!f.source || f.source === "built-in") return NATIVE_ENGINE;
  return f.source;
}

function sameExt(a: string, b: string): boolean {
  return a.toLowerCase() === b.toLowerCase();
}

/**
 * The engines with a reader for any of the named format's extensions, native
 * first and the plugins in name order. One entry means no choice to make.
 */
export function enginesForFormat(formats: FormatInfo[], name: string): string[] {
  const target = formats.find((f) => f.name === name);
  if (!target?.extensions?.length) return [];
  const engines = new Set<string>();
  for (const f of formats) {
    if (!f.has_reader || !f.extensions?.length) continue;
    if (f.extensions.some((e) => target.extensions!.some((te) => sameExt(e, te)))) {
      engines.add(engineOf(f));
    }
  }
  return [...engines].sort((a, b) => {
    if (a === NATIVE_ENGINE) return -1;
    if (b === NATIVE_ENGINE) return 1;
    return a.localeCompare(b);
  });
}

/**
 * The format of `engine` for the logical format `name` names: `name` itself
 * when it belongs to the engine, else the engine's reader claiming the most of
 * `name`'s extensions, with priority and then name order breaking a tie (the
 * same rule as the registry's EngineFormat). Undefined when the engine has
 * none.
 */
export function engineFormatFor(
  formats: FormatInfo[],
  name: string,
  engine: string,
): string | undefined {
  const target = formats.find((f) => f.name === name);
  if (!target) return undefined;
  if (engineOf(target) === engine) return name;
  const exts = target.extensions ?? [];
  let best: { name: string; overlap: number; priority: number } | undefined;
  for (const f of formats) {
    if (engineOf(f) !== engine || !f.has_reader) continue;
    const overlap = (f.extensions ?? []).filter((e) => exts.some((te) => sameExt(e, te))).length;
    if (overlap === 0) continue;
    const priority = f.priority ?? 0;
    if (
      !best ||
      overlap > best.overlap ||
      (overlap === best.overlap &&
        (priority > best.priority || (priority === best.priority && f.name < best.name)))
    ) {
      best = { name: f.name, overlap, priority };
    }
  }
  return best?.name;
}
