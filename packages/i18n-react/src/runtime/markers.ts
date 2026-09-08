/**
 * The tokens a compiled message carries.
 *
 * A block that holds inline JSX flattens to a template where each inline
 * element is a marker: `{=mN}` opens a pair (or stands alone when no matching
 * close follows in the same scope) and `{/=mN}` closes one. `__tx` walks these
 * to rebuild the React tree; a runtime string transform walks them to find out
 * which characters belong to a span whose text is a command or a key rather
 * than prose.
 *
 * A block's other children flatten to `{name}` placeholders. Most of those
 * carry text and substitute into the message before anything walks it, but a
 * parameter whose value turns out to be a React element keeps its token so
 * `__tx` can put the element itself in the output. `collectValueTokens` finds
 * those, and `collectTokens` merges both scans into one positional list.
 *
 * Everything here needs the same pairing rule, so it lives in one place.
 */

export interface MarkerToken {
  /** Index of the token's first character in the message. */
  start: number;
  /** Index one past the token's last character. */
  end: number;
  /** Marker name (`=mN`) for a pair, or the placeholder name for a value. */
  key: string;
  kind: "open" | "close" | "value";
}

/**
 * The translator's answer for the text inside each paired element, keyed by
 * marker name. `"no"` means the span holds a command, a key the reader
 * presses, sample output or an identifier; `"yes"` means an explicit
 * `translate="yes"` opted the span back in. A marker with no entry inherits
 * the answer of whatever encloses it.
 *
 * The compiler fills this from the same rule the extractor applies when it
 * marks a run `noTranslate`, so a runtime transform and the catalog build
 * protect the same bytes.
 */
export type MarkerTranslate = Readonly<Record<string, "yes" | "no">>;

const MARKER_RE = /\{(\/?)(=[^}]+)\}/g;

/**
 * Scan `text` for element marker tokens, returning them in positional order.
 */
export function collectMarkerTokens(text: string): MarkerToken[] {
  const tokens: MarkerToken[] = [];
  MARKER_RE.lastIndex = 0;
  let m: RegExpExecArray | null;
  while ((m = MARKER_RE.exec(text)) !== null) {
    tokens.push({
      start: m.index,
      end: m.index + m[0].length,
      key: m[2],
      kind: m[1] === "/" ? "close" : "open",
    });
  }
  return tokens;
}

/**
 * Scan `text` for the `{name}` token of every name in `names`, returning them
 * in positional order. `__tx` asks for the parameters whose value holds React
 * content: those keep their token through substitution so the renderer can
 * emit the node rather than its `String()` form.
 *
 * A name that appears more than once yields one token per appearance, and a
 * name absent from the message yields none, which is what lets a translation
 * move or drop a placeholder.
 */
export function collectValueTokens(text: string, names: readonly string[]): MarkerToken[] {
  if (names.length === 0) return [];
  const pattern = new RegExp(`\\{(${names.map(escapeForRegExp).join("|")})\\}`, "g");
  const tokens: MarkerToken[] = [];
  let m: RegExpExecArray | null;
  while ((m = pattern.exec(text)) !== null) {
    tokens.push({ start: m.index, end: m.index + m[0].length, key: m[1], kind: "value" });
  }
  return tokens;
}

/**
 * Every token in `text` in positional order: the element markers plus the
 * `{name}` tokens for `valueNames`. A placeholder name never begins with `=`,
 * so the two scans claim disjoint bytes.
 */
export function collectTokens(text: string, valueNames: readonly string[]): MarkerToken[] {
  const markers = collectMarkerTokens(text);
  if (valueNames.length === 0) return markers;
  const merged = markers.concat(collectValueTokens(text, valueNames));
  merged.sort((a, b) => a.start - b.start);
  return merged;
}

function escapeForRegExp(name: string): string {
  return name.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

/**
 * Match opens with closes, LIFO: a close pops the topmost open carrying the
 * same key. Returns the index of the matching close for every open that has
 * one; an open with no entry is standalone, and a close that matched nothing
 * is absent from the values.
 */
export function pairMarkers(tokens: readonly MarkerToken[]): Map<number, number> {
  const closeOf = new Map<number, number>();
  const openStack: number[] = [];
  for (let i = 0; i < tokens.length; i++) {
    const tok = tokens[i];
    if (tok.kind === "value") continue;
    if (tok.kind === "open") {
      openStack.push(i);
      continue;
    }
    for (let j = openStack.length - 1; j >= 0; j--) {
      if (tokens[openStack[j]].key === tok.key) {
        closeOf.set(openStack[j], i);
        openStack.splice(j, 1);
        break;
      }
    }
  }
  return closeOf;
}

/**
 * Per-character protection for `text`, one entry per UTF-16 code unit: `true`
 * where the character sits inside a paired element whose answer is `"no"`.
 * Returns `null` when nothing is protected, which is the common case and lets
 * a caller skip the whole question.
 *
 * Nesting follows the extractor: a pair with no answer of its own inherits the
 * enclosing one, so a `<span>` inside a `<code>` stays protected, and a
 * `translate="yes"` inside a `<code>` opens a hole the transform may write in.
 * A marker token's own characters take the answer of the scope it sits in, so
 * `{=m0}` reads as the parent's and `{/=m0}` likewise.
 */
export function protectionMask(
  text: string,
  markers: MarkerTranslate | undefined,
): boolean[] | null {
  if (!markers) return null;
  let anyProtected = false;
  for (const answer of Object.values(markers)) {
    if (answer === "no") anyProtected = true;
  }
  if (!anyProtected) return null;

  const tokens = collectMarkerTokens(text);
  if (tokens.length === 0) return null;
  const closeOf = pairMarkers(tokens);
  const openOfClose = new Map<number, number>();
  for (const [open, close] of closeOf) openOfClose.set(close, open);

  const mask: boolean[] = Array.from({ length: text.length }, () => false);
  const stack: boolean[] = [];
  const current = () => (stack.length > 0 ? stack[stack.length - 1] : false);
  let cursor = 0;

  const fill = (from: number, to: number, value: boolean) => {
    if (!value) return;
    for (let i = from; i < to; i++) mask[i] = true;
  };

  for (let i = 0; i < tokens.length; i++) {
    const tok = tokens[i];
    fill(cursor, tok.start, current());
    if (tok.kind === "open" && closeOf.has(i)) {
      fill(tok.start, tok.end, current());
      const answer = markers[tok.key];
      stack.push(answer === undefined ? current() : answer === "no");
    } else if (tok.kind === "close" && openOfClose.has(i)) {
      stack.pop();
      fill(tok.start, tok.end, current());
    } else {
      fill(tok.start, tok.end, current());
    }
    cursor = tok.end;
  }
  fill(cursor, text.length, current());

  return mask.includes(true) ? mask : null;
}
