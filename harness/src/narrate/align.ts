/**
 * Align a narration script against the transcript of its audio.
 *
 * A one-shot narration is one recording of every scene's text read in order.
 * The transcript (whisper.cpp, token timestamps) says when each word was
 * spoken; this module says which scene each spoken word belongs to, so the
 * scene boundaries in the audio are measured rather than guessed from word
 * counts. The alignment is a global sequence alignment over normalized words,
 * which tolerates the transcript hearing "kapi" as "copy", dropping a short
 * word, or splitting a compound.
 */

export interface SpokenWord {
  text: string;
  startMs: number;
  endMs: number;
}

export interface SceneSpan {
  /** First aligned word's start. */
  startMs: number;
  /** Last aligned word's end. */
  endMs: number;
  /** Script words of the scene that found a spoken word. */
  aligned: number;
  /** Script words of the scene. */
  total: number;
}

export interface Alignment {
  /** One span per scene; null for a scene none of whose words were found. */
  spans: (SceneSpan | null)[];
  /** Aligned script words over all script words, 0 to 1. */
  coverage: number;
}

/** Lower-case, letters and digits only, so punctuation and case never count. */
export function normalizeToken(t: string): string {
  return t
    .toLowerCase()
    .normalize("NFKD")
    .replace(/[^\p{L}\p{N}]+/gu, "");
}

/** Split text into normalized word tokens, dropping the empties. */
export function tokenize(text: string): string[] {
  return text
    .split(/\s+/)
    .map(normalizeToken)
    .filter((t) => t.length > 0);
}

function editDistance(a: string, b: string): number {
  const m = a.length;
  const n = b.length;
  if (m === 0) return n;
  if (n === 0) return m;
  let prev = new Array<number>(n + 1);
  let cur = new Array<number>(n + 1);
  for (let j = 0; j <= n; j++) prev[j] = j;
  for (let i = 1; i <= m; i++) {
    cur[0] = i;
    for (let j = 1; j <= n; j++) {
      const cost = a[i - 1] === b[j - 1] ? 0 : 1;
      cur[j] = Math.min(prev[j]! + 1, cur[j - 1]! + 1, prev[j - 1]! + cost);
    }
    [prev, cur] = [cur, prev];
  }
  return prev[n]!;
}

/** How alike two normalized words are: 2 exact, 1 close, -1 different. */
function similarity(a: string, b: string): number {
  if (a === b) return 2;
  if (a.length >= 4 && b.length >= 4) {
    if (a.startsWith(b) || b.startsWith(a)) return 1;
    if (editDistance(a, b) <= Math.max(1, Math.floor(Math.min(a.length, b.length) / 4))) return 1;
  }
  return -1;
}

const GAP = -1;

/**
 * Global alignment (Needleman-Wunsch) of the script words against the spoken
 * words. Returns, for every script word, the index of the spoken word it
 * matched or -1.
 */
function alignWords(script: string[], spoken: string[]): number[] {
  const m = script.length;
  const n = spoken.length;
  // score[i][j]: best score aligning script[0..i) with spoken[0..j).
  const score: Int32Array[] = [];
  const move: Uint8Array[] = []; // 0 diag, 1 up (script gap), 2 left (spoken gap)
  for (let i = 0; i <= m; i++) {
    score.push(new Int32Array(n + 1));
    move.push(new Uint8Array(n + 1));
  }
  for (let i = 1; i <= m; i++) {
    score[i]![0] = i * GAP;
    move[i]![0] = 1;
  }
  for (let j = 1; j <= n; j++) {
    score[0]![j] = j * GAP;
    move[0]![j] = 2;
  }
  for (let i = 1; i <= m; i++) {
    const si = script[i - 1]!;
    for (let j = 1; j <= n; j++) {
      const diag = score[i - 1]![j - 1]! + similarity(si, spoken[j - 1]!);
      const up = score[i - 1]![j]! + GAP;
      const left = score[i]![j - 1]! + GAP;
      let best = diag;
      let mv = 0;
      if (up > best) {
        best = up;
        mv = 1;
      }
      if (left > best) {
        best = left;
        mv = 2;
      }
      score[i]![j] = best;
      move[i]![j] = mv;
    }
  }
  const out = new Array<number>(m).fill(-1);
  let i = m;
  let j = n;
  while (i > 0 && j > 0) {
    const mv = move[i]![j];
    if (mv === 0) {
      if (similarity(script[i - 1]!, spoken[j - 1]!) > 0) out[i - 1] = j - 1;
      i--;
      j--;
    } else if (mv === 1) {
      i--;
    } else {
      j--;
    }
  }
  return out;
}

/**
 * Find each scene's span in the spoken words. Scene texts are aligned as one
 * script in order; a scene whose words were all missed gets a null span and
 * the caller interpolates it.
 */
export function alignScenes(sceneTexts: string[], words: SpokenWord[]): Alignment {
  const script: string[] = [];
  const owner: number[] = [];
  sceneTexts.forEach((text, k) => {
    for (const t of tokenize(text)) {
      script.push(t);
      owner.push(k);
    }
  });
  const spoken = words.map((w) => normalizeToken(w.text));
  const matched = alignWords(script, spoken);
  const spans: (SceneSpan | null)[] = sceneTexts.map(() => null);
  const totals = sceneTexts.map(() => 0);
  let aligned = 0;
  matched.forEach((j, i) => {
    const k = owner[i]!;
    totals[k]!++;
    if (j < 0) return;
    aligned++;
    const w = words[j]!;
    const cur = spans[k];
    if (!cur) spans[k] = { startMs: w.startMs, endMs: w.endMs, aligned: 1, total: 0 };
    else {
      cur.startMs = Math.min(cur.startMs, w.startMs);
      cur.endMs = Math.max(cur.endMs, w.endMs);
      cur.aligned++;
    }
  });
  spans.forEach((s, k) => {
    if (s) s.total = totals[k]!;
  });
  return { spans, coverage: script.length > 0 ? aligned / script.length : 0 };
}

/** The lead a cut takes before the first word of a scene, at most. */
const CUT_LEAD_MS = 200;

/**
 * Turn scene spans into cut points in the audio: one boundary per scene
 * start, the first at 0, and the end of the whole track. A boundary sits just
 * before the scene's first word, inside the pause that precedes it, so the
 * picture changes as the sentence starts. Scenes with no span split the room
 * between their neighbours by word count.
 */
export function sceneBoundaries(spans: (SceneSpan | null)[], wordCounts: number[], totalMs: number): number[] {
  const n = spans.length;
  const starts = new Array<number>(n).fill(NaN);
  const known = (k: number): SceneSpan | null => spans[k] ?? null;
  for (let k = 0; k < n; k++) {
    const s = known(k);
    if (!s) continue;
    if (k === 0) {
      starts[k] = 0;
      continue;
    }
    const prev = known(k - 1);
    const gap = prev ? s.startMs - prev.endMs : CUT_LEAD_MS * 2;
    starts[k] = gap > 0 ? s.startMs - Math.min(CUT_LEAD_MS, gap / 2) : s.startMs;
  }
  starts[0] = 0;
  // Fill unknown runs by word share of the room between the known neighbours.
  let k = 1;
  while (k < n) {
    if (!Number.isNaN(starts[k]!)) {
      k++;
      continue;
    }
    let end = k;
    while (end < n && Number.isNaN(starts[end]!)) end++;
    const roomStart = known(k - 1)?.endMs ?? starts[k - 1]!;
    const roomEnd = end < n ? starts[end]! : totalMs;
    const room = Math.max(0, roomEnd - roomStart);
    const total = wordCounts.slice(k - 1, end).reduce((a, b) => a + b, 0) || 1;
    let acc = wordCounts[k - 1]!;
    for (let i = k; i < end; i++) {
      starts[i] = roomStart + (room * acc) / total;
      acc += wordCounts[i]!;
    }
    k = end;
  }
  // Monotone and inside the track.
  for (let i = 1; i < n; i++) starts[i] = Math.min(totalMs, Math.max(starts[i]!, starts[i - 1]!));
  return [...starts, totalMs];
}
