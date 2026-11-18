/**
 * A word-level diff between two run sequences, for showing a fix against the
 * draft it changes. Inline codes are tokens of their own and compare by id, so
 * a moved or dropped placeholder shows as a change.
 */

import {
  codeDisplays,
  projectRuns,
  type CodeDisplay,
  type ModelRunSpec,
  type Run,
} from "@neokapi/kapi-format";

/**
 * One unit a diff compares: a word, the space between words, or a code. A
 * code carries its display (never its data), which is what a surface draws.
 */
export type DiffToken =
  | { kind: "text"; text: string }
  | { kind: "code"; id: string; display: CodeDisplay; role: "ph" | "open" | "close" | "sub" }
  | { kind: "branch"; display: CodeDisplay; key: string };

/** One span of a diff. */
export interface DiffSpan {
  op: "same" | "add" | "del";
  token: DiffToken;
}

function tokenSpec(runs: readonly Run[]): ModelRunSpec<DiffToken[]> {
  const code = codeDisplays(runs);
  return {
    text: (r) => splitWords(r.text).map((text) => ({ kind: "text", text })),
    ph: ({ ph }) => [{ kind: "code", id: ph.id, display: code.ph(ph), role: "ph" }],
    pcOpen: ({ pcOpen }) => [
      { kind: "code", id: pcOpen.id, display: code.pcOpen(pcOpen), role: "open" },
    ],
    pcClose: ({ pcClose }) => [
      { kind: "code", id: pcClose.id, display: code.pcClose(pcClose), role: "close" },
    ],
    sub: ({ sub }) => [{ kind: "code", id: sub.id, display: code.sub(sub), role: "sub" }],
    // A plural or select is one token: its forms compare as a whole.
    plural: ({ plural }) => [
      { kind: "branch", display: code.choice(plural.pivot), key: JSON.stringify(plural) },
    ],
    select: ({ select }) => [
      { kind: "branch", display: code.choice(select.pivot), key: JSON.stringify(select) },
    ],
    fallback: (kind) => [{ kind: "branch", display: code.unknown(), key: kind }],
  };
}

/** The tokens of a run sequence, in document order. */
export function runTokens(runs: readonly Run[]): DiffToken[] {
  return projectRuns(runs, tokenSpec(runs)).flat();
}

function splitWords(text: string): string[] {
  return text.split(/(\s+)/).filter((s) => s !== "");
}

function tokenKey(t: DiffToken): string {
  switch (t.kind) {
    case "text":
      return `t:${t.text}`;
    case "code":
      return `c:${t.role}:${t.id}`;
    case "branch":
      return `b:${t.key}`;
  }
}

/** Diff two run sequences by longest common subsequence over their tokens. */
export function diffRuns(before: readonly Run[], after: readonly Run[]): DiffSpan[] {
  const a = runTokens(before);
  const b = runTokens(after);
  const ka = a.map(tokenKey);
  const kb = b.map(tokenKey);
  const n = a.length;
  const m = b.length;
  // lcs[i][j] is the common length of a[i..] and b[j..].
  const lcs: number[][] = Array.from({ length: n + 1 }, () =>
    Array.from({ length: m + 1 }, () => 0),
  );
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      lcs[i][j] = ka[i] === kb[j] ? lcs[i + 1][j + 1] + 1 : Math.max(lcs[i + 1][j], lcs[i][j + 1]);
    }
  }
  const out: DiffSpan[] = [];
  let i = 0;
  let j = 0;
  while (i < n && j < m) {
    if (ka[i] === kb[j]) {
      out.push({ op: "same", token: b[j] });
      i++;
      j++;
    } else if (lcs[i + 1][j] >= lcs[i][j + 1]) {
      out.push({ op: "del", token: a[i++] });
    } else {
      out.push({ op: "add", token: b[j++] });
    }
  }
  while (i < n) out.push({ op: "del", token: a[i++] });
  while (j < m) out.push({ op: "add", token: b[j++] });
  return out;
}

/** Whether two run sequences hold the same content. */
export function sameRuns(a: readonly Run[], b: readonly Run[]): boolean {
  return diffRuns(a, b).every((s) => s.op === "same");
}
