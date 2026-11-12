/**
 * A word-level diff between two run sequences, for showing a fix against the
 * draft it changes. Inline codes are tokens of their own and compare by id, so
 * a moved or dropped placeholder shows as a change.
 */

import { projectRuns, type ModelRunSpec, type Run } from "@neokapi/kapi-format";

/** One unit a diff compares: a word, the space between words, or a code. */
export type DiffToken =
  | { kind: "text"; text: string }
  | { kind: "code"; id: string; label: string; role: "ph" | "open" | "close" | "sub" }
  | { kind: "branch"; label: string; key: string };

/** One span of a diff. */
export interface DiffSpan {
  op: "same" | "add" | "del";
  token: DiffToken;
}

const TOKENS: ModelRunSpec<DiffToken[]> = {
  text: (r) => splitWords(r.text).map((text) => ({ kind: "text", text })),
  ph: ({ ph }) => [{ kind: "code", id: ph.id, label: ph.disp || ph.equiv || ph.id, role: "ph" }],
  pcOpen: ({ pcOpen }) => [
    { kind: "code", id: pcOpen.id, label: pcOpen.disp || pcOpen.equiv || pcOpen.id, role: "open" },
  ],
  pcClose: ({ pcClose }) => [
    { kind: "code", id: pcClose.id, label: pcClose.equiv || pcClose.id, role: "close" },
  ],
  sub: ({ sub }) => [{ kind: "code", id: sub.id, label: sub.equiv || sub.id, role: "sub" }],
  // A plural or select is one token: its forms compare as a whole.
  plural: ({ plural }) => [
    { kind: "branch", label: `{${plural.pivot}, plural}`, key: JSON.stringify(plural) },
  ],
  select: ({ select }) => [
    { kind: "branch", label: `{${select.pivot}, select}`, key: JSON.stringify(select) },
  ],
  fallback: (kind) => [{ kind: "branch", label: `{${kind}}`, key: kind }],
};

/** The tokens of a run sequence, in document order. */
export function runTokens(runs: readonly Run[]): DiffToken[] {
  return projectRuns(runs, TOKENS).flat();
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
  const lcs: number[][] = Array.from({ length: n + 1 }, () => new Array<number>(m + 1).fill(0));
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
