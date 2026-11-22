// Splits a rule's regex source into tokens a reader can tell apart: groups,
// alternatives, repeats, escapes, character sets and anchors. Display only:
// the server and kapi match the regex, and this never decides what matches.

export type RegexTokenKind =
  | "literal"
  | "escape"
  | "set"
  | "group"
  | "alternative"
  | "repeat"
  | "anchor";

export interface RegexToken {
  kind: RegexTokenKind;
  text: string;
}

/** The tokens of a regex source, which concatenate back to the source exactly. */
export function regexTokens(source: string): RegexToken[] {
  const out: RegexToken[] = [];
  const push = (kind: RegexTokenKind, text: string) => {
    const last = out[out.length - 1];
    if (kind === "literal" && last?.kind === "literal") last.text += text;
    else out.push({ kind, text });
  };
  let i = 0;
  while (i < source.length) {
    const c = source[i];
    if (c === "\\") {
      push("escape", source.slice(i, i + 2));
      i += 2;
    } else if (c === "[") {
      // A set runs to the first unescaped "]" after its first character.
      let j = i + 1;
      if (source[j] === "^") j++;
      if (source[j] === "]") j++;
      while (j < source.length && source[j] !== "]") j += source[j] === "\\" ? 2 : 1;
      push("set", source.slice(i, j + 1));
      i = j + 1;
    } else if (c === "(") {
      // "(?:", "(?i)", "(?P<name>" read as part of the bracket.
      const m = /^\((\?(?:[A-Za-z]*:|[A-Za-z]+\)|P?<[^>]*>|[=!]|<[=!]))?/.exec(source.slice(i));
      const text = m?.[0] ?? "(";
      push("group", text);
      i += text.length;
    } else if (c === ")") {
      push("group", c);
      i++;
    } else if (c === "|") {
      push("alternative", c);
      i++;
    } else if (c === "*" || c === "+" || c === "?") {
      // A lazy or possessive mark belongs to its repeat.
      const text = source[i + 1] === "?" || source[i + 1] === "+" ? source.slice(i, i + 2) : c;
      push("repeat", text);
      i += text.length;
    } else if (c === "{") {
      const m = /^\{\d*(,\d*)?\}\??/.exec(source.slice(i));
      if (m) {
        push("repeat", m[0]);
        i += m[0].length;
      } else {
        push("literal", c);
        i++;
      }
    } else if (c === "^" || c === "$") {
      push("anchor", c);
      i++;
    } else {
      push("literal", c);
      i++;
    }
  }
  return out;
}
