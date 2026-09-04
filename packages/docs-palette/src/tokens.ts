/**
 * Reading a canonical palette file.
 *
 * The brand palettes are plain CSS: a `:root` block for light, a `.dark` block
 * for dark, and an `@import` of the shared semantic tokens. Parsing that needs
 * no CSS engine, and using one would let a preprocessor decide what the palette
 * says. This reads exactly the two blocks and the custom properties in them.
 */

/** One theme's custom properties, keyed without the leading `--`. */
export type TokenMap = Map<string, string>;

export interface CanonicalPalette {
  light: TokenMap;
  dark: TokenMap;
}

const COMMENTS = /\/\*[\s\S]*?\*\//g;
const IMPORTS = /@import\s+(?:url\()?["']([^"']+)["']\)?\s*;/g;
const DECLARATION = /--([A-Za-z0-9_-]+)\s*:\s*([^;}]+)[;}]/g;

/** Strip block comments so a commented-out declaration is not read as one. */
export function stripComments(css: string): string {
  return css.replace(COMMENTS, "");
}

/** The index of the `}` closing the `{` at `open`, or -1 if the file is truncated. */
function matchingBrace(css: string, open: number): number {
  let depth = 0;
  for (let i = open; i < css.length; i++) {
    if (css[i] === "{") depth++;
    else if (css[i] === "}" && --depth === 0) return i;
  }
  return -1;
}

/** The `@import` specifiers in a stylesheet, in source order. */
export function importedFiles(css: string): string[] {
  const out: string[] = [];
  for (const match of stripComments(css).matchAll(IMPORTS)) out.push(match[1]);
  return out;
}

/**
 * The declarations inside the outermost blocks whose selector is `selector`.
 * A file may open the same selector more than once (the palettes do, to keep a
 * later group of tokens beside the rules that use them); later declarations win,
 * as they do in the cascade.
 */
export function readBlock(css: string, selector: string): TokenMap {
  const body = stripComments(css);
  const tokens: TokenMap = new Map();
  let cursor = 0;
  while (cursor < body.length) {
    const start = body.indexOf(selector, cursor);
    if (start === -1) break;
    const open = body.indexOf("{", start);
    if (open === -1) break;
    const between = body.slice(start + selector.length, open).trim();
    const preceding = start === 0 ? "" : body[start - 1];
    // `.dark` must be the whole selector, not the tail of `.foo.dark` or the
    // head of `.darker`, and nothing but whitespace may sit before the brace.
    if (between !== "" || /[A-Za-z0-9_.#[-]/.test(preceding)) {
      cursor = start + selector.length;
      continue;
    }
    const close = matchingBrace(body, open);
    if (close === -1) break;
    const block = body.slice(open + 1, close + 1);
    for (const match of block.matchAll(DECLARATION)) {
      // A value may be wrapped across lines in the source (a long font stack);
      // the bridge writes one line per declaration, so fold the run of
      // whitespace a line break leaves behind.
      tokens.set(match[1], match[2].trim().replace(/\s+/g, " "));
    }
    cursor = close + 1;
  }
  return tokens;
}

/** Both themes of one stylesheet, ignoring anything it imports. */
export function readPalette(css: string): CanonicalPalette {
  return { light: readBlock(css, ":root"), dark: readBlock(css, ".dark") };
}

/** `later` wins over `earlier`, the way a later declaration wins in the cascade. */
export function mergePalettes(
  earlier: CanonicalPalette,
  later: CanonicalPalette,
): CanonicalPalette {
  return {
    light: new Map([...earlier.light, ...later.light]),
    dark: new Map([...earlier.dark, ...later.dark]),
  };
}
