/**
 * When each word of a desktop beat's narration is spoken, so a walk can act
 * as the narration names the act rather than at a fixed delay.
 *
 * The narrate stage writes narration.json (each scene with the beat it is
 * spoken over) and captions.json (each scene's words with their start, in
 * milliseconds from the scene's start). A scene plays its beat's slice from
 * the beat's start, so a word's start is also its moment in the beat.
 */
import fs from "node:fs";
import path from "node:path";

/** One word of a beat's narration and when it starts, in seconds from the beat's start. */
export interface Cue {
  word: string;
  at: number;
}

interface NarrationFile {
  scenes?: Array<{ id: string; kind: string; beat?: string }>;
}

interface CaptionsFile {
  scenes?: Array<{ id: string; captions?: Array<{ text: string; startMs: number }> }>;
}

/** A word as cues compare it: lower case, without the punctuation around it. */
export function cueWord(word: string): string {
  return word.toLowerCase().replace(/^[^\p{L}\p{N}]+|[^\p{L}\p{N}]+$/gu, "");
}

/** Beat id → the words spoken over it, in order, from a parsed narration and its captions. */
export function cuesFrom(narration: NarrationFile, captions: CaptionsFile): Record<string, Cue[]> {
  const beatOf = new Map<string, string>();
  for (const s of narration.scenes ?? []) {
    if (s.kind === "desktop" && s.beat) beatOf.set(s.id, s.beat);
  }
  const out: Record<string, Cue[]> = {};
  for (const scene of captions.scenes ?? []) {
    const beat = beatOf.get(scene.id);
    if (!beat) continue;
    out[beat] = (scene.captions ?? []).map((w) => ({ word: cueWord(w.text), at: w.startMs / 1000 }));
  }
  return out;
}

/** When `word` is first spoken over a beat, or undefined when it is not. */
export function cueAt(cues: Cue[] | undefined, word: string): number | undefined {
  const want = cueWord(word);
  return cues?.find((c) => c.word === want)?.at;
}

/**
 * The cues of a narration synthesized before this recording, read from the
 * demo's public directory. Empty when either file is missing or unreadable,
 * so a walk falls back to its own seconds.
 */
export function narrationCues(outDir: string, uiLocale?: string): Record<string, Cue[]> {
  const suffix = uiLocale ? `-${uiLocale}` : "";
  const narration = path.join(outDir, `narration${suffix}.json`);
  const captions = path.join(outDir, `captions${suffix}.json`);
  if (!fs.existsSync(narration) || !fs.existsSync(captions)) return {};
  try {
    return cuesFrom(
      JSON.parse(fs.readFileSync(narration, "utf8")) as NarrationFile,
      JSON.parse(fs.readFileSync(captions, "utf8")) as CaptionsFile,
    );
  } catch {
    return {};
  }
}
