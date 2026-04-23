/**
 * Parse an ICU `{pivot, plural, form {content} …}` string back into
 * a `PluralRun`. Round-trips with `flattenRuns` for plural targets,
 * so translators can reopen a saved plural target and keep editing
 * in the per-form view.
 *
 * Deliberately narrow: only top-level plural. If the string isn't a
 * plural construct, `parseICUPluralString` returns `null` and the
 * caller falls back to flat-text parsing (`runsToText` etc.). The
 * select path is covered by Track A's developer-authored `<Select>`
 * components — translator-authored select is rare enough to add
 * later if demand appears.
 *
 * Grammar:
 *   message       := '{' WS pivot WS ',' WS 'plural' WS ',' WS forms WS '}'
 *   forms         := form (WS form)*
 *   form          := form-name WS '{' content '}'
 *   form-name     := 'zero' | 'one' | 'two' | 'few' | 'many' | 'other'
 *                  | '=' integer                (ICU explicit-case, accepted but normalised to 'other' for safety)
 *   content       := characters-with-balanced-braces
 *
 * Nested braces inside content are respected via depth tracking so
 * placeholder tokens (`{count}`) and paired codes (`{=m0}`) survive.
 */

import type { PluralForm, PluralRunWrapper, Run } from "./block.ts";

/**
 * Parses content within a plural form into a Run sequence. Callers
 * typically pass a bound `textToRuns` that knows the block's
 * placeholder table. When omitted, content becomes a single
 * `TextRun` — still a valid `Run[]`, just without placeholder typing.
 */
export type ContentParser = (text: string) => Run[];

const CLDR_FORMS: ReadonlySet<string> = new Set(["zero", "one", "two", "few", "many", "other"]);

/**
 * Try to parse `text` as an ICU plural message. Returns a
 * single-element `Run[]` containing the `PluralRunWrapper`, or
 * `null` if the grammar doesn't match.
 *
 * The parser deliberately rejects anything outside `{pivot, plural,
 * …}` — `select` and non-ICU strings fall through to `null` so the
 * caller can handle them with plain-text parsing.
 */
export function parseICUPluralString(text: string, parseContent?: ContentParser): Run[] | null {
  const wrapper = parsePluralMessage(text.trim(), parseContent);
  return wrapper ? [wrapper] : null;
}

function parsePluralMessage(text: string, parseContent?: ContentParser): PluralRunWrapper | null {
  if (text.length < 2 || text[0] !== "{" || text[text.length - 1] !== "}") {
    return null;
  }

  // Strip outer braces; everything else happens on the inner payload.
  const inner = text.slice(1, -1);

  const firstComma = indexOfTopLevel(inner, ",", 0);
  if (firstComma < 0) return null;
  const secondComma = indexOfTopLevel(inner, ",", firstComma + 1);
  if (secondComma < 0) return null;

  const pivot = inner.slice(0, firstComma).trim();
  const kind = inner.slice(firstComma + 1, secondComma).trim();
  const tail = inner.slice(secondComma + 1).trim();
  if (!pivot || kind !== "plural" || !tail) return null;

  const forms = parseForms(tail, parseContent);
  if (!forms) return null;

  return { plural: { pivot, forms } };
}

function parseForms(
  tail: string,
  parseContent: ContentParser | undefined,
): Partial<Record<PluralForm, Run[]>> | null {
  const forms: Partial<Record<PluralForm, Run[]>> = {};
  let i = 0;

  while (i < tail.length) {
    // Skip whitespace between forms.
    while (i < tail.length && /\s/.test(tail[i])) i++;
    if (i >= tail.length) break;

    // Read form name up to the next `{`.
    const braceStart = tail.indexOf("{", i);
    if (braceStart < 0) return null;
    const rawName = tail.slice(i, braceStart).trim();
    const form = normalizeFormName(rawName);
    if (!form) return null;

    // Find the matching `}` for this content, respecting nesting.
    const braceEnd = findMatchingBrace(tail, braceStart);
    if (braceEnd < 0) return null;

    const content = tail.slice(braceStart + 1, braceEnd);
    forms[form] = parseContent ? parseContent(content) : [{ text: content }];
    i = braceEnd + 1;
  }

  if (Object.keys(forms).length === 0) return null;
  return forms;
}

/**
 * ICU accepts both keyword forms (`one`, `other`) and explicit numeric
 * cases (`=0`, `=1`). The typed model only models CLDR keywords, so
 * explicit cases map onto `one` when sensible and `other` otherwise.
 * This is lossy on purpose — translators who need exact numeric
 * casing are better served by `<Select>` with integer-valued pivots.
 */
function normalizeFormName(name: string): PluralForm | null {
  if (CLDR_FORMS.has(name)) return name as PluralForm;
  if (name === "=0") return "zero";
  if (name === "=1") return "one";
  if (name === "=2") return "two";
  // Any other `=N` collapses to `other`; explicit numeric cases
  // beyond those three are rare and we'd rather preserve content
  // than silently drop a form.
  if (/^=\d+$/.test(name)) return "other";
  return null;
}

/**
 * Find the index of the first `}` that matches the `{` at `openAt`,
 * respecting brace nesting. Returns -1 if the braces are unbalanced.
 */
function findMatchingBrace(text: string, openAt: number): number {
  if (text[openAt] !== "{") return -1;
  let depth = 0;
  for (let i = openAt; i < text.length; i++) {
    if (text[i] === "{") depth++;
    else if (text[i] === "}") {
      depth--;
      if (depth === 0) return i;
    }
  }
  return -1;
}

/**
 * Find `needle` in `text` starting at `from`, but only count
 * occurrences at top-level brace depth (depth === 0). Used to split
 * a message header on its first two commas without confusing commas
 * inside placeholder tokens or nested messages.
 */
function indexOfTopLevel(text: string, needle: string, from: number): number {
  let depth = 0;
  for (let i = from; i < text.length; i++) {
    const ch = text[i];
    if (ch === "{") depth++;
    else if (ch === "}") depth--;
    else if (depth === 0 && ch === needle) return i;
  }
  return -1;
}
