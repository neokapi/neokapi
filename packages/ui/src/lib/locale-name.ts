/**
 * Locale display names.
 *
 * A locale code is an identifier, and a reader needs a label. `fr` names the
 * language to the machine and to a reviewer who already knows the tag; everyone
 * else reads a two-letter abbreviation and guesses. So a surface with room for a
 * name shows one, and a surface without room (a dense coverage grid, a table
 * cell) shows the code and keeps the name reachable, which is what `LocalePill`
 * does with its title.
 *
 * CLDR names every well-formed tag, so this needs no per-locale table and
 * handles a real project target (`fr-FR`, `pt-BR`, `zh-Hant`) as readily as a
 * major tag. It falls back to the code when the runtime has no `Intl.DisplayNames`
 * or has no name for the tag (`qps`, our pseudo locale, echoes its own code back).
 */

let intlNames: Intl.DisplayNames | null = null;

/** Resolve a locale code to an English display name, falling back to the code. */
export function resolveLocaleName(code: string): string {
  if (!code) return code;
  try {
    if (!intlNames) intlNames = new Intl.DisplayNames("en", { type: "language" });
    return intlNames.of(code) ?? code;
  } catch {
    return code;
  }
}

/**
 * Render a locale as "Name (code)" for a single-string context (a tooltip, an
 * aria-label, a title) where a pill and a name cannot sit side by side. When the
 * code has no name it is returned alone rather than doubled.
 */
export function localeLabel(code: string): string {
  const name = resolveLocaleName(code);
  return name === code ? code : `${name} (${code})`;
}
