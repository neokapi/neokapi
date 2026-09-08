/**
 * Captions from the narration audio, and the scene spans a one-shot track is
 * cut into.
 *
 * whisper.cpp transcribes each narration file with token timestamps; the
 * tokens become the timed captions the composition pages in its lower third.
 * For a one-shot narration (one continuous read of every scene) the same
 * transcript is aligned against the script to find where each scene's words
 * begin, so the scene boundaries are measured from the audio rather than
 * guessed from word counts.
 *
 * whisper.cpp and its model are installed once, outside the repo, under the
 * user's cache directory (HARNESS_WHISPER_DIR overrides).
 */
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { downloadWhisperModel, installWhisperCpp, toCaptions, transcribe, type Language, type WhisperModel } from "@remotion/install-whisper-cpp";
import type { CaptionsFile, NarrationScene, TimedCaption } from "../types.ts";
import { run } from "../lib/exec.ts";
import { isDefaultLocale, localeSuffix } from "../lib/locale.ts";
import { alignScenes, sceneBoundaries, tokenize } from "./align.ts";

/** The whisper.cpp release the harness builds; needs cmake on this machine. */
export const WHISPER_CPP_VERSION = "1.7.6";

/** Alignment coverage below which a one-shot track keeps its word-share cuts. */
const MIN_ALIGNMENT_COVERAGE = 0.6;

/** Transcription can be switched off for a machine that cannot build whisper.cpp. */
export function captionsEnabled(): boolean {
  return (process.env.HARNESS_CAPTIONS ?? "1") !== "0";
}

/** Where whisper.cpp and its models live: the user's cache, never the repo. */
export function whisperDir(): string {
  if (process.env.HARNESS_WHISPER_DIR) return path.resolve(process.env.HARNESS_WHISPER_DIR);
  const cache = process.env.XDG_CACHE_HOME || path.join(os.homedir(), ".cache");
  return path.join(cache, "neokapi", "harness", "whisper.cpp");
}

/** The model per narration language: the English-only small model for English, the multilingual one otherwise. */
export function whisperModelFor(locale: string): WhisperModel {
  const forced = process.env.HARNESS_WHISPER_MODEL as WhisperModel | undefined;
  if (forced) return forced;
  return isDefaultLocale(locale) ? "small.en" : "small";
}

/** whisper.cpp's language code for a narration locale (nb and nn are "no" to it). */
export function whisperLanguage(locale: string): Language {
  const code = locale.toLowerCase().split("-")[0] ?? "en";
  if (code === "nb" || code === "nn") return "no";
  return code as Language;
}

let installed: Promise<string> | null = null;

/** Install whisper.cpp and the model once per process; return the install folder. */
async function ensureWhisper(model: WhisperModel, log: (msg: string) => void): Promise<string> {
  if (!installed) {
    installed = (async () => {
      const to = whisperDir();
      fs.mkdirSync(path.dirname(to), { recursive: true });
      const { alreadyExisted } = await installWhisperCpp({ to, version: WHISPER_CPP_VERSION, printOutput: false });
      if (!alreadyExisted) log(`installed whisper.cpp ${WHISPER_CPP_VERSION} into ${to}`);
      return to;
    })();
  }
  const to = await installed;
  const { alreadyExisted } = await downloadWhisperModel({ model, folder: to, printOutput: false });
  if (!alreadyExisted) log(`downloaded whisper model ${model}`);
  return to;
}

/** whisper.cpp reads 16 kHz 16-bit mono WAV only; the narration is 24 kHz. */
async function to16k(wav: string): Promise<string> {
  const out = wav.replace(/\.wav$/, "") + ".16k.wav";
  const r = await run("ffmpeg", ["-y", "-i", wav, "-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", out], { timeoutMs: 120_000 });
  if (r.code !== 0) throw new Error(`ffmpeg resample failed: ${r.stderr.slice(-400)}`);
  return out;
}

/** Transcribe one narration WAV into timed word captions (ms from its start). */
export async function transcribeWav(wav: string, locale: string, log: (msg: string) => void): Promise<TimedCaption[]> {
  const model = whisperModelFor(locale);
  const to = await ensureWhisper(model, log);
  const input = await to16k(wav);
  try {
    const out = await transcribe({
      inputPath: input,
      whisperPath: to,
      whisperCppVersion: WHISPER_CPP_VERSION,
      model,
      tokenLevelTimestamps: true,
      printOutput: false,
      language: whisperLanguage(locale),
      splitOnWord: true,
    });
    const { captions } = toCaptions({ whisperCppOutput: out });
    return captions.map((c) => ({ text: c.text, startMs: c.startMs, endMs: c.endMs, timestampMs: c.timestampMs, confidence: c.confidence }));
  } finally {
    fs.rmSync(input, { force: true });
  }
}

/** The engine string a captions file records. */
export function captionEngine(locale: string): string {
  return `whisper.cpp ${WHISPER_CPP_VERSION} ${whisperModelFor(locale)}`;
}

export interface NarrationDraft {
  scenes: NarrationScene[];
  /** staticFile-relative one-shot track, when the narration is one read. */
  fullAudio?: string;
}

export interface CaptionContext {
  id: string;
  locale: string;
  /** public/<id> */
  publicDir: string;
  log: (msg: string) => void;
}

export interface CaptionResult {
  scenes: NarrationScene[];
  sceneTiming: "measured" | "word-share";
  /** staticFile-relative captions file, when transcription ran. */
  captions?: string;
}

/**
 * Word-share cut points for a one-shot track: each spoken scene takes the
 * share of the track its words are of the whole. The fallback when the
 * transcript could not be aligned, and the cut used with captions off.
 */
export function wordShareBoundaries(texts: string[], totalMs: number): number[] {
  const counts = texts.map((t) => tokenize(t).length);
  const total = counts.reduce((a, b) => a + b, 0) || 1;
  const out: number[] = [0];
  let acc = 0;
  for (let i = 0; i < counts.length; i++) {
    acc += counts[i]!;
    out.push((totalMs * acc) / total);
  }
  return out;
}

/** Captions with a start inside [fromMs, toMs), re-based to fromMs. */
export function sliceCaptions(all: TimedCaption[], fromMs: number, toMs: number): TimedCaption[] {
  return all
    .filter((c) => c.startMs >= fromMs && c.startMs < toMs)
    .map((c) => ({ ...c, startMs: c.startMs - fromMs, endMs: Math.min(c.endMs, toMs) - fromMs, timestampMs: c.timestampMs === null ? null : c.timestampMs - fromMs }));
}

/** Apply cut points to the spoken scenes of a one-shot draft, in place. */
function applyBoundaries(spokenIdx: number[], scenes: NarrationScene[], boundariesMs: number[]): void {
  spokenIdx.forEach((sceneIdx, k) => {
    const s = scenes[sceneIdx]!;
    const from = boundariesMs[k]!;
    const to = boundariesMs[k + 1]!;
    s.audioFrom = from / 1000;
    s.audioTo = to / 1000;
    s.durationSec = (to - from) / 1000;
  });
}

/**
 * Finish a narration: transcribe its audio into captions.json and, for a
 * one-shot track, cut the track into measured scene spans. With captions
 * off (HARNESS_CAPTIONS=0) a one-shot track is cut by word share and no
 * captions file is written.
 */
export async function attachCaptions(draft: NarrationDraft, ctx: CaptionContext): Promise<CaptionResult> {
  const scenes = draft.scenes.map((s) => ({ ...s }));
  const spokenIdx = scenes.map((s, i) => (s.text.trim() ? i : -1)).filter((i) => i >= 0);
  const suffix = localeSuffix(ctx.locale);
  const captionsRel = `captions${suffix}.json`;
  const captionsPath = path.join(ctx.publicDir, captionsRel);
  const file: CaptionsFile = { id: ctx.id, engine: captionEngine(ctx.locale), scenes: [] };
  if (!isDefaultLocale(ctx.locale)) file.locale = ctx.locale;

  if (draft.fullAudio) {
    const wav = path.join(ctx.publicDir, draft.fullAudio);
    const totalMs = Math.round(wavDurationSec(wav) * 1000);
    const texts = spokenIdx.map((i) => scenes[i]!.text);
    let boundaries = wordShareBoundaries(texts, totalMs);
    let sceneTiming: CaptionResult["sceneTiming"] = "word-share";
    if (!captionsEnabled()) {
      ctx.log("captions off (HARNESS_CAPTIONS=0): one-shot scenes cut by word share");
      applyBoundaries(spokenIdx, scenes, boundaries);
      return { scenes, sceneTiming };
    }
    const all = await transcribeWav(wav, ctx.locale, ctx.log);
    const { spans, coverage } = alignScenes(texts, all);
    if (coverage >= MIN_ALIGNMENT_COVERAGE) {
      boundaries = sceneBoundaries(
        spans,
        texts.map((t) => tokenize(t).length),
        totalMs,
      );
      sceneTiming = "measured";
      ctx.log(`aligned ${Math.round(coverage * 100)}% of the script to the transcript; scene cuts measured`);
    } else {
      ctx.log(`! aligned only ${Math.round(coverage * 100)}% of the script to the transcript; scene cuts by word share`);
    }
    applyBoundaries(spokenIdx, scenes, boundaries);
    spokenIdx.forEach((sceneIdx, k) => {
      file.scenes.push({ id: scenes[sceneIdx]!.id, captions: sliceCaptions(all, boundaries[k]!, boundaries[k + 1]!) });
    });
    fs.writeFileSync(captionsPath, JSON.stringify(file, null, 2));
    ctx.log(`captions: ${file.scenes.reduce((n, s) => n + s.captions.length, 0)} words over ${file.scenes.length} scenes`);
    return { scenes, sceneTiming, captions: captionsRel };
  }

  // Per-scene clips: every clip starts at its scene's start, so its transcript
  // is already scene-relative.
  if (!captionsEnabled()) {
    ctx.log("captions off (HARNESS_CAPTIONS=0)");
    return { scenes, sceneTiming: "measured" };
  }
  for (const i of spokenIdx) {
    const s = scenes[i]!;
    if (!s.audio) continue;
    const captions = await transcribeWav(path.join(ctx.publicDir, s.audio), ctx.locale, ctx.log);
    file.scenes.push({ id: s.id, captions });
  }
  fs.writeFileSync(captionsPath, JSON.stringify(file, null, 2));
  ctx.log(`captions: ${file.scenes.reduce((n, s) => n + s.captions.length, 0)} words over ${file.scenes.length} scenes`);
  return { scenes, sceneTiming: "measured", captions: captionsRel };
}

/** Read duration (seconds) from a canonical PCM WAV file. */
export function wavDurationSec(file: string): number {
  const buf = fs.readFileSync(file);
  // Walk chunks to find fmt + data (robust to extra chunks).
  let offset = 12;
  let sampleRate = 0;
  let channels = 1;
  let bits = 16;
  let dataLen = 0;
  while (offset + 8 <= buf.length) {
    const id = buf.toString("ascii", offset, offset + 4);
    const size = buf.readUInt32LE(offset + 4);
    if (id === "fmt ") {
      channels = buf.readUInt16LE(offset + 10);
      sampleRate = buf.readUInt32LE(offset + 12);
      bits = buf.readUInt16LE(offset + 22);
    } else if (id === "data") {
      dataLen = size;
    }
    offset += 8 + size + (size % 2);
  }
  if (!sampleRate) return 0;
  return dataLen / (sampleRate * channels * (bits / 8));
}
