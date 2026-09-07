/**
 * Element markers in a compiled message.
 *
 * A block that carries inline JSX flattens to a template where each inline
 * element is a token: `{=mN}` opens a pair (or stands alone when no matching
 * close follows in the same scope) and `{/=mN}` closes one. `__tx` walks these
 * to rebuild the React tree; a runtime string transform walks them to find out
 * which characters belong to a span whose text is a command or a key rather
 * than prose.
 *
 * Both need the same pairing rule, so it lives here once.
 */

export interface MarkerToken {
  /** Index of the token's first character in the message. */
  start: number;
  /** Index one past the token's last character. */
  end: number;
  /** Marker name, `=mN`. */
  key: string;
  kind: "open" | "close";
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

  const mask = new Array<boolean>(text.length).fill(false);
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
