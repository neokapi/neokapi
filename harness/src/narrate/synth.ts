import fs from "node:fs";
import path from "node:path";
import { Buffer } from "node:buffer";
import type { DemoManifest, NarrationManifest, NarrationScene, NarrationSpec } from "../types.ts";
import { ensureDir, publicDemoDir } from "../lib/paths.ts";
import { run } from "../lib/exec.ts";
import { DEFAULT_LOCALE, isDefaultLocale, languageNameFor, localeSuffix, localizeManifest, resolveLocale } from "../lib/locale.ts";

// ── Locale-aware narrator style ─────────────────────────────────────────────
// The product-name pronunciation hints are kept in EVERY locale's prompt: the
// names are never translated, and the Gemini TTS model applies the respelling
// regardless of the narration language.

const PRONUNCIATION_HINTS =
  "Always pronounce the " +
  "product name \"kapi\" as KAH-pee — two syllables, stress on the first, the 'a' as in 'father' " +
  "— never ka-PEE or kap-ee. Always pronounce the product name \"Bowrain\" as BOH-rain — the " +
  "first syllable \"bow\" rhymes with \"rainbow\" and \"go\" (NOT \"cow\" or bowing down), " +
  "followed by \"rain\"; it is a blend of \"rainbow\" and \"rain\"";

/** Style directive prepended to every Gemini TTS request: a clear documentary
 *  narrator in the narration locale's language (British English by default). */
function geminiStyle(locale: string): string {
  const lang = languageNameFor(locale);
  const voiceLine = isDefaultLocale(locale)
    ? "Read this aloud in a clear, warm, professional British-English documentary-narrator voice, "
    : `Read this aloud in ${lang}, as a clear, warm, professional ${lang} documentary narrator ` +
      "— natural native pronunciation, never an English accent. Keep product names, CLI commands " +
      "and code identifiers exactly as written. Speak ";
  return (
    voiceLine +
    "at a measured, unhurried, and consistent pace — keep the exact same tone, energy, and tempo " +
    "from the first word to the last. Leave a clear pause between paragraphs. " +
    PRONUNCIATION_HINTS + ": "
  );
}

type Backend = "gemini" | "elevenlabs" | "openai" | "say";

/** A voice env var with per-locale override: GEMINI_TTS_VOICE_NB beats GEMINI_TTS_VOICE for nb. */
function voiceEnv(base: string, locale: string): string | undefined {
  if (!isDefaultLocale(locale)) {
    const v = process.env[`${base}_${locale.toUpperCase().replace(/-/g, "_")}`];
    if (v) return v;
  }
  return process.env[base];
}

function pickBackend(locale: string = DEFAULT_LOCALE): { backend: Backend; voice: string } {
  const forced = (process.env.NARRATION_BACKEND || "").toLowerCase() as Backend;
  if (forced === "elevenlabs" || (!forced && process.env.ELEVENLABS_API_KEY)) {
    if (process.env.ELEVENLABS_API_KEY) {
      return { backend: "elevenlabs", voice: voiceEnv("ELEVENLABS_VOICE_ID", locale) || "Rachel" };
    }
  }
  if (forced === "openai" || (!forced && process.env.OPENAI_API_KEY && !process.env.GEMINI_API_KEY)) {
    if (process.env.OPENAI_API_KEY) {
      return { backend: "openai", voice: voiceEnv("OPENAI_TTS_VOICE", locale) || "onyx" };
    }
  }
  if ((forced === "gemini" || !forced) && process.env.GEMINI_API_KEY) {
    // Gemini's prebuilt voices are language-agnostic (the language follows the
    // text + style prompt), so the default voice works for every locale; pin a
    // different one per locale with GEMINI_TTS_VOICE_<LOCALE> (e.g. _NB).
    return { backend: "gemini", voice: voiceEnv("GEMINI_TTS_VOICE", locale) || "Charon" };
  }
  if (forced === "gemini" && !process.env.GEMINI_API_KEY) {
    // Explicit gemini request with no key: fail loudly rather than silently
    // degrading to the offline `say` voice.
    throw new Error(
      "NARRATION_BACKEND=gemini but GEMINI_API_KEY is unset — add it to harness/.env (no `say` fallback when gemini is explicitly requested)",
    );
  }
  // macOS `say`: the bundled voices are single-language, so a per-locale voice
  // is REQUIRED for non-English narration (e.g. SAY_VOICE_NB="Nora").
  return { backend: "say", voice: voiceEnv("SAY_VOICE", locale) || "Daniel" };
}

// ── WAV helpers ─────────────────────────────────────────────────────────────

function pcmToWav(pcm: Buffer, sampleRate: number, channels = 1, bitsPerSample = 16): Buffer {
  const byteRate = (sampleRate * channels * bitsPerSample) / 8;
  const blockAlign = (channels * bitsPerSample) / 8;
  const header = Buffer.alloc(44);
  header.write("RIFF", 0);
  header.writeUInt32LE(36 + pcm.length, 4);
  header.write("WAVE", 8);
  header.write("fmt ", 12);
  header.writeUInt32LE(16, 16);
  header.writeUInt16LE(1, 20); // PCM
  header.writeUInt16LE(channels, 22);
  header.writeUInt32LE(sampleRate, 24);
  header.writeUInt32LE(byteRate, 28);
  header.writeUInt16LE(blockAlign, 32);
  header.writeUInt16LE(bitsPerSample, 34);
  header.write("data", 36);
  header.writeUInt32LE(pcm.length, 40);
  return Buffer.concat([header, pcm]);
}

/** Read duration (seconds) from a canonical PCM WAV file. */
function wavDurationSec(file: string): number {
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

async function toWav(input: string, output: string): Promise<void> {
  // Re-encode anything (mp3/aiff/…) to canonical 24 kHz mono 16-bit PCM WAV.
  const r = await run("ffmpeg", ["-y", "-i", input, "-ar", "24000", "-ac", "1", "-c:a", "pcm_s16le", output], {
    timeoutMs: 60_000,
  });
  if (r.code !== 0) throw new Error(`ffmpeg failed: ${r.stderr.slice(-400)}`);
}

// ── Narration delivery tuning ────────────────────────────────────────────────
// Committed code constants — NOT environment-driven. These govern the narrator's
// pace and tone, so they must be identical on every machine and in CI; keeping
// them in .env made the delivery inconsistent. .env is for secrets only
// (GEMINI_API_KEY) + operational toggles (backend selection), never voice tuning.

/** Per-scene fallback pace (atempo, pitch-preserving). 1.3 = noticeably brisker;
 *  per-scene pace is equalized around it so scenes don't drift faster/slower. */
const NARRATION_SPEED = 1.3;
/** Gemini Live pace. The Live model already reads at ~150 wpm, so no acceleration
 *  (1.0); this only equalizes speed across scenes. */
const NARRATION_LIVE_SPEED = 1.0;
/** Gemini TTS sampling temperature. Lower = steadier tone across scenes (0.4 =
 *  "stable" per Gemini's docs). */
const GEMINI_TEMPERATURE = 0.4;

function narrationSpeed(): number {
  return NARRATION_SPEED;
}

function narrationLiveSpeed(): number {
  return NARRATION_LIVE_SPEED;
}

function geminiTemperature(): number {
  return GEMINI_TEMPERATURE;
}

/** Speed a WAV in place by an explicit atempo factor (pitch-preserving). */
async function applyTempo(wav: string, factor: number): Promise<void> {
  if (Math.abs(factor - 1) < 0.001) return;
  const tmp = wav + ".speed.wav";
  const r = await run("ffmpeg", ["-y", "-i", wav, "-filter:a", `atempo=${factor.toFixed(3)}`, "-ar", "24000", "-ac", "1", "-c:a", "pcm_s16le", tmp], {
    timeoutMs: 60_000,
  });
  if (r.code !== 0) throw new Error(`ffmpeg atempo failed: ${r.stderr.slice(-400)}`);
  fs.renameSync(tmp, wav);
}

function countWords(text: string): number {
  return text.trim().split(/\s+/).filter(Boolean).length;
}

function median(nums: number[]): number {
  if (nums.length === 0) return 0;
  const s = [...nums].sort((a, b) => a - b);
  const mid = Math.floor(s.length / 2);
  return s.length % 2 ? s[mid] : (s[mid - 1] + s[mid]) / 2;
}

// ── Backends ────────────────────────────────────────────────────────────────

async function geminiTts(text: string, voice: string, outWav: string, locale: string): Promise<void> {
  const key = process.env.GEMINI_API_KEY!;
  const model = process.env.GEMINI_TTS_MODEL || "gemini-3.1-flash-tts-preview";
  const url = `https://generativelanguage.googleapis.com/v1beta/models/${model}:generateContent?key=${key}`;
  const body = {
    contents: [{ parts: [{ text: geminiStyle(locale) + text }] }],
    generationConfig: {
      responseModalities: ["AUDIO"],
      // Lower temperature = steadier delivery between scenes. Each scene is a separate
      // request, and this generative TTS model otherwise re-rolls its tone/energy per call;
      // Gemini's docs put "stable" around 0.4 and "lively" at 0.7+. Pin it so the narrator
      // doesn't shift voice/tone scene-to-scene. Override with NARRATION_TEMPERATURE.
      temperature: geminiTemperature(),
      speechConfig: { voiceConfig: { prebuiltVoiceConfig: { voiceName: voice } } },
    },
  };
  const resp = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!resp.ok) throw new Error(`Gemini TTS ${resp.status}: ${(await resp.text()).slice(0, 300)}`);
  const data: any = await resp.json();
  const part = data?.candidates?.[0]?.content?.parts?.find((p: any) => p.inlineData);
  if (!part) throw new Error("Gemini TTS: no audio in response");
  const mime: string = part.inlineData.mimeType || "";
  const rateMatch = mime.match(/rate=(\d+)/);
  const sampleRate = rateMatch ? Number(rateMatch[1]) : 24000;
  const pcm = Buffer.from(part.inlineData.data, "base64");
  fs.writeFileSync(outWav, pcmToWav(pcm, sampleRate));
}

async function elevenLabsTts(text: string, voice: string, outWav: string): Promise<void> {
  const key = process.env.ELEVENLABS_API_KEY!;
  const model = process.env.ELEVENLABS_MODEL || "eleven_multilingual_v2";
  const url = `https://api.elevenlabs.io/v1/text-to-speech/${voice}`;
  const resp = await fetch(url, {
    method: "POST",
    headers: { "xi-api-key": key, "Content-Type": "application/json", Accept: "audio/mpeg" },
    body: JSON.stringify({ text, model_id: model, voice_settings: { stability: 0.5, similarity_boost: 0.75 } }),
  });
  if (!resp.ok) throw new Error(`ElevenLabs ${resp.status}: ${(await resp.text()).slice(0, 300)}`);
  const mp3 = Buffer.from(await resp.arrayBuffer());
  const tmp = outWav + ".mp3";
  fs.writeFileSync(tmp, mp3);
  await toWav(tmp, outWav);
  fs.rmSync(tmp, { force: true });
}

async function openaiTts(text: string, voice: string, outWav: string): Promise<void> {
  const key = process.env.OPENAI_API_KEY!;
  const model = process.env.OPENAI_TTS_MODEL || "gpt-4o-mini-tts";
  const resp = await fetch("https://api.openai.com/v1/audio/speech", {
    method: "POST",
    headers: { Authorization: `Bearer ${key}`, "Content-Type": "application/json" },
    body: JSON.stringify({ model, voice, input: text, response_format: "wav" }),
  });
  if (!resp.ok) throw new Error(`OpenAI TTS ${resp.status}: ${(await resp.text()).slice(0, 300)}`);
  const wav = Buffer.from(await resp.arrayBuffer());
  const tmp = outWav + ".raw.wav";
  fs.writeFileSync(tmp, wav);
  await toWav(tmp, outWav); // normalize sample rate
  fs.rmSync(tmp, { force: true });
}

async function sayTts(text: string, voice: string, outWav: string): Promise<void> {
  // macOS `say` rejects `--data-format=LEI16@…` with the default AIFF container
  // ("Opening output file failed: fmt?"), since AIFF is big-endian. Pin the WAVE
  // container so the little-endian PCM format is valid; toWav then normalizes.
  const tmp = outWav + ".say.wav";
  const r = await run(
    "say",
    ["-v", voice, "-o", tmp, "--file-format=WAVE", "--data-format=LEI16@24000", text],
    { timeoutMs: 60_000 },
  );
  if (r.code !== 0) throw new Error(`say failed: ${r.stderr.slice(-300)}`);
  await toWav(tmp, outWav);
  fs.rmSync(tmp, { force: true });
}

async function synthOne(backend: Backend, voice: string, text: string, outWav: string, locale: string): Promise<void> {
  // Pronunciation of "kapi" is handled by the style prompt (geminiStyle), not by
  // respelling the word — respelling it ("kah-pee") made the voice say "ka-PEE".
  switch (backend) {
    case "gemini":
      return geminiTts(text, voice, outWav, locale);
    case "elevenlabs":
      return elevenLabsTts(text, voice, outWav);
    case "openai":
      return openaiTts(text, voice, outWav);
    case "say":
      return sayTts(text, voice, outWav);
  }
}

// ── Live-session narration ──────────────────────────────────────────────────

/**
 * System instruction that turns a conversational Live model into a strict verbatim
 * narrator, in the narration locale's language. Validated against the output
 * transcript: the model reads each turn word-for-word rather than replying to it.
 */
function liveNarratorInstruction(locale: string): string {
  const lang = languageNameFor(locale);
  const voiceLine = isDefaultLocale(locale)
    ? "in a clear, warm, measured British-English voice at a steady, unhurried pace. "
    : `in a clear, warm, measured ${lang} voice at a steady, unhurried pace — natural native ` +
      `${lang} pronunciation, never an English accent; product names, CLI commands and code ` +
      "identifiers stay exactly as written. ";
  return (
    "You are a documentary narrator. Read the user's text aloud VERBATIM, word for word, " +
    voiceLine +
    "Do not " +
    "reply, greet, comment, summarise, translate, or add or drop any words — narrate exactly " +
    'what is given. Pronounce the product name "kapi" as KAH-pee (two syllables, stress on ' +
    "the first, 'a' as in 'father'), never ka-PEE or kap-ee. Pronounce the product name " +
    '"Bowrain" as BOH-rain — the first syllable "bow" rhymes with "rainbow" and "go" (NOT ' +
    '"cow" or bowing down), then "rain"; it is a blend of "rainbow" and "rain".'
  );
}

/**
 * Narrate every scene in ONE Gemini Live (bidi) session. The model holds a single
 * voice and persona across turns, so the narration can't drift scene-to-scene the way
 * independent per-scene calls do — and there's no synthesize-then-split: one turn per
 * scene yields one clip per scene, read verbatim. Writes each scene's WAV and resolves
 * with the per-scene durations, in order. Rejects on protocol error / empty audio so
 * the caller falls back to per-scene synthesis.
 */
function geminiLiveNarrate(
  voice: string,
  scenes: Array<{ text: string; outWav: string }>,
  model: string,
  locale: string,
): Promise<number[]> {
  const key = process.env.GEMINI_API_KEY;
  if (!key) return Promise.reject(new Error("live: GEMINI_API_KEY unset"));
  const url = `wss://generativelanguage.googleapis.com/ws/google.ai.generativelanguage.v1beta.GenerativeService.BidiGenerateContent?key=${key}`;

  return new Promise<number[]>((resolve, reject) => {
    const ws = new WebSocket(url);
    ws.binaryType = "arraybuffer";
    let idx = -1;
    let rate = 24000;
    let chunks: Buffer[] = [];
    const durations: number[] = [];
    let settled = false;

    const finish = (fn: () => void) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      try {
        ws.close();
      } catch {
        /* already closed */
      }
      fn();
    };
    const fail = (msg: string) => finish(() => reject(new Error(`live: ${msg}`)));
    // Generous budget scaled by total content, not scene COUNT: a one-shot read is
    // a single long turn whose audio is as long as all scenes combined, so a
    // count-based budget under-provisions it (and timed out). ~150ms/char + setup.
    const totalChars = scenes.reduce((s, sc) => s + sc.text.length, 0);
    const timer = setTimeout(() => fail("session timeout"), 30_000 + totalChars * 150);

    const sendTurn = (i: number) =>
      ws.send(JSON.stringify({ clientContent: { turns: [{ role: "user", parts: [{ text: scenes[i].text }] }], turnComplete: true } }));

    ws.onopen = () =>
      ws.send(
        JSON.stringify({
          setup: {
            model: `models/${model}`,
            generationConfig: { responseModalities: ["AUDIO"], speechConfig: { voiceConfig: { prebuiltVoiceConfig: { voiceName: voice } } } },
            systemInstruction: { parts: [{ text: liveNarratorInstruction(locale) }] },
          },
        }),
      );

    ws.onmessage = async (ev: MessageEvent) => {
      let data: unknown = ev.data;
      if (data instanceof ArrayBuffer) data = Buffer.from(data).toString("utf8");
      else if (data instanceof Blob) data = await data.text();
      let msg: {
        setupComplete?: unknown;
        serverContent?: {
          modelTurn?: { parts?: Array<{ inlineData?: { data?: string; mimeType?: string } }> };
          turnComplete?: boolean;
        };
      };
      try {
        msg = JSON.parse(data as string);
      } catch {
        return;
      }
      if (msg.setupComplete) {
        idx = 0;
        chunks = [];
        sendTurn(0);
        return;
      }
      const sc = msg.serverContent;
      if (!sc) return;
      for (const p of sc.modelTurn?.parts ?? []) {
        if (p.inlineData?.data) {
          const m = /rate=(\d+)/.exec(p.inlineData.mimeType ?? "");
          if (m) rate = Number(m[1]);
          chunks.push(Buffer.from(p.inlineData.data, "base64"));
        }
      }
      if (sc.turnComplete) {
        const pcm = Buffer.concat(chunks);
        if (pcm.length === 0) return fail(`empty audio for scene ${idx}`);
        fs.writeFileSync(scenes[idx].outWav, pcmToWav(pcm, rate));
        durations.push(wavDurationSec(scenes[idx].outWav));
        chunks = [];
        idx++;
        if (idx >= scenes.length) finish(() => resolve(durations));
        else sendTurn(idx);
      }
    };
    ws.onerror = () => fail("websocket error");
    ws.onclose = (e: CloseEvent) => {
      if (!settled) fail(`websocket closed (code ${e.code}${e.reason ? ` ${e.reason}` : ""})`);
    };
  });
}

// ── Orchestration ─────────────────────────────────────────────────────────────

export interface NarrateOptions {
  force?: boolean;
  /** BCP-47 narration locale (default "en"). Non-default locales synthesize
   *  from the demo's `locales.<locale>` overlay into suffixed outputs
   *  (narration-<locale>.json + audio-<locale>/). */
  locale?: string;
}

/** Synthesize narration for every scene → public/<id>/audio[-<locale>]/*.wav + narration[-<locale>].json. */
export async function narrateDemo(manifest: DemoManifest, opts: NarrateOptions = {}): Promise<NarrationManifest> {
  const locale = resolveLocale(opts.locale);
  const suffix = localeSuffix(locale);
  // Apply the locale's narration overlay (throws for published demos without
  // full coverage). The default locale passes the manifest through untouched.
  const m = localizeManifest(manifest, locale);
  const audioRel = `audio${suffix}`;
  const pub = publicDemoDir(m.id);
  const audioDir = ensureDir(path.join(pub, audioRel));
  const narrationPath = path.join(pub, `narration${suffix}.json`);

  if (!opts.force && fs.existsSync(narrationPath)) {
    console.log(`  · narration exists for ${m.id}${suffix} (use --force to re-run)`);
    return JSON.parse(fs.readFileSync(narrationPath, "utf8"));
  }

  const { backend, voice } = pickBackend(locale);
  const localeNote = isDefaultLocale(locale) ? "" : ` [${locale}]`;
  console.log(`  · narrating ${m.id}${localeNote} with ${backend} (${voice}), ${m.narration.length} scenes`);

  // One-shot: narrate the WHOLE video in a single continuous read, so the voice's
  // tempo and tone can't drift scene-to-scene (no per-scene clips, no per-scene
  // tempo normalization). Scene durations are word-proportional shares of the one
  // track — exact enough because a single read holds a near-constant words/sec.
  // DEFAULT for gemini narration so every re-record is voice-consistent by
  // construction; disable per demo with `oneshot: false` or globally with
  // NARRATION_ONESHOT=0 (then it falls through to the Live/per-scene paths).
  const oneshotEnabled = m.oneshot ?? ((process.env.NARRATION_ONESHOT ?? "1") !== "0");
  if (backend === "gemini" && oneshotEnabled) {
    const oneShot = await narrateOneShot(m, voice, audioDir, narrationPath, backend, locale, audioRel);
    if (oneShot) return oneShot;
    console.warn("  ! one-shot narration failed; falling back to per-scene");
  }

  // Preferred path: narrate the whole video in ONE Gemini Live session — the model
  // holds one voice/persona across turns (no scene-to-scene drift), one turn per scene
  // gives clean per-scene clips, and it's read verbatim. Falls through to per-scene
  // synthesis if the session fails. Gemini only — other backends are steady already.
  const spoken = m.narration.filter((s) => s.text?.trim());
  const useLive = (process.env.NARRATION_LIVE ?? "1") !== "0";
  if (backend === "gemini" && useLive && spoken.length >= 1) {
    try {
      const liveModel = process.env.GEMINI_LIVE_MODEL || "gemini-3.1-flash-live-preview";
      const inputs = spoken.map((s) => ({ text: s.text.trim(), outWav: path.join(audioDir, `${s.id}.wav`) }));
      const rawDurs = await geminiLiveNarrate(voice, inputs, liveModel, locale);

      // The session already gives a consistent voice; normalize *speed* so every
      // scene reads at the SAME words-per-second (the median × NARRATION_LIVE_SPEED).
      // The clamp is deliberately WIDE: it bounds how far each clip can be nudged,
      // so a tight band would *under*-correct outliers and leave audible
      // scene-to-scene drift (the prior ±20-25% band did exactly that). A clip from
      // one Live session sits near the median anyway, so the wide band fully
      // equalizes ordinary variation and only guards against atempo artefacts on a
      // pathological clip.
      const speed = narrationLiveSpeed();
      const words = spoken.map((s) => countWords(s.text));
      const naturalWps = rawDurs.map((d, i) => (d > 0 ? words[i] / d : 0)).filter((w) => w > 0);
      const targetWps = median(naturalWps) * speed;
      const minF = speed * 0.6;
      const maxF = speed * 1.7;
      const finalDur = new Map<string, number>();
      for (let i = 0; i < spoken.length; i++) {
        const wps = rawDurs[i] > 0 ? words[i] / rawDurs[i] : 0;
        const factor = wps > 0 && targetWps > 0 ? Math.min(maxF, Math.max(minF, targetWps / wps)) : speed;
        await applyTempo(inputs[i].outWav, factor);
        finalDur.set(spoken[i].id, wavDurationSec(inputs[i].outWav));
      }

      const scenes: NarrationScene[] = m.narration.map((spec) => {
        const text = spec.text?.trim() ?? "";
        return {
          id: spec.id,
          kind: spec.kind,
          text,
          caption: spec.caption?.trim() || captionFromText(spec.text),
          artifact: spec.artifact,
          beat: spec.beat,
          audio: text ? `${audioRel}/${spec.id}.wav` : undefined,
          durationSec: text ? (finalDur.get(spec.id) ?? 0) : 0,
          holdSec: spec.holdSec ?? defaultHold(spec.kind),
        };
      });
      const result: NarrationManifest = { id: m.id, backend, voice, scenes };
      if (!isDefaultLocale(locale)) result.locale = locale;
      fs.writeFileSync(narrationPath, JSON.stringify(result, null, 2));
      const total = scenes.reduce((s, sc) => s + sc.durationSec + sc.holdSec, 0);
      console.log(`  ✓ narrated ${m.id}${localeNote} (live session): ${scenes.length} scenes, ${total.toFixed(1)}s total audio`);
      return result;
    } catch (e) {
      console.warn(`  ! live-session narration failed (${(e as Error).message.slice(0, 120)}); falling back to per-scene`);
    }
  }

  // Pass 1 — synthesize every spoken scene at the model's natural rate. We defer the
  // tempo step so we can equalize pace across scenes afterwards: a flat per-scene
  // multiplier preserves whatever rate each independent TTS call happened to choose,
  // which is the main source of scene-to-scene "speed" drift.
  interface Synthed {
    spec: NarrationSpec;
    wavName: string;
    outWav: string;
    spoken: boolean;
    words: number;
    naturalDur: number;
  }
  const synthed: Synthed[] = [];
  for (const spec of m.narration) {
    const wavName = `${spec.id}.wav`;
    const outWav = path.join(audioDir, wavName);
    const text = spec.text?.trim() ?? "";
    if (text) {
      const words = countWords(text);
      let attempt = 0;
      let naturalDur = 0;
      // The Gemini preview TTS model is flaky: under load it intermittently returns a
      // fast empty-audio 200, a 500 INTERNAL, or — worse, because it looks like success —
      // a pathological looped/garbled clip that's wildly too long for the text. These
      // misses are quick and random (~50% during a bad patch), so the winning strategy is
      // many quick re-tries with only a gentle backoff. Budget via NARRATION_TTS_RETRIES.
      const maxRetries = Math.max(1, Number(process.env.NARRATION_TTS_RETRIES) || 10);
      while (true) {
        try {
          await synthOne(backend, voice, text, outWav, locale);
          naturalDur = wavDurationSec(outWav);
          // Reject pathological clips (the looped-audio failure mode): a real narrator
          // lands ~1.5–4 w/s, so anything outside a generous 0.7–12 w/s band for a
          // non-trivial line is a bad generation — re-roll it like any other miss.
          const wps = naturalDur > 0 ? words / naturalDur : 0;
          if (words >= 3 && (wps < 0.7 || wps > 12)) {
            throw new Error(`implausible audio: ${naturalDur.toFixed(0)}s for ${words} words (${wps.toFixed(2)} w/s)`);
          }
          break;
        } catch (e) {
          attempt++;
          if (attempt >= maxRetries) throw e;
          const backoff = Math.min(12_000, 700 * 2 ** (attempt - 1));
          console.warn(`    retry ${attempt}/${maxRetries} in ${(backoff / 1000).toFixed(1)}s (${(e as Error).message.slice(0, 100)})`);
          await new Promise((r) => setTimeout(r, backoff));
        }
      }
      synthed.push({ spec, wavName, outWav, spoken: true, words, naturalDur });
    } else {
      synthed.push({ spec, wavName, outWav, spoken: false, words: 0, naturalDur: 0 });
    }
  }

  // Pass 2 — target one words-per-second pace for the whole video: the median natural
  // pace scaled by NARRATION_SPEED. Each scene is nudged toward it (clamped to ±20–25%
  // of the global speed so no clip is stretched into artefacts) so the narrator sounds
  // equally brisk throughout instead of speeding up and slowing down between scenes.
  const speed = narrationSpeed();
  const naturalWps = synthed.filter((s) => s.spoken && s.naturalDur > 0).map((s) => s.words / s.naturalDur);
  const targetWps = median(naturalWps) * speed;
  // Wide band so outlier scenes are pulled fully to the shared pace (see the
  // live-session path above for why a tight clamp under-corrects). Cap below the
  // single-stage atempo ceiling (2.0×).
  const minF = speed * 0.6;
  const maxF = Math.min(2.0, speed * 1.5);

  const scenes: NarrationScene[] = [];
  for (const s of synthed) {
    let durationSec = 0;
    let audio: string | undefined;
    if (s.spoken) {
      const wps = s.naturalDur > 0 ? s.words / s.naturalDur : 0;
      const factor = wps > 0 && targetWps > 0 ? Math.min(maxF, Math.max(minF, targetWps / wps)) : speed;
      await applyTempo(s.outWav, factor);
      durationSec = wavDurationSec(s.outWav);
      audio = `${audioRel}/${s.wavName}`;
    }
    scenes.push({
      id: s.spec.id,
      kind: s.spec.kind,
      text: s.spec.text.trim(),
      caption: s.spec.caption?.trim() || captionFromText(s.spec.text),
      artifact: s.spec.artifact,
      beat: s.spec.beat,
      audio,
      durationSec,
      holdSec: s.spec.holdSec ?? defaultHold(s.spec.kind),
    });
  }

  const result: NarrationManifest = { id: m.id, backend, voice, scenes };
  if (!isDefaultLocale(locale)) result.locale = locale;
  fs.writeFileSync(narrationPath, JSON.stringify(result, null, 2));
  const total = scenes.reduce((s, sc) => s + sc.durationSec + sc.holdSec, 0);
  console.log(`  ✓ narrated ${m.id}${localeNote}: ${scenes.length} scenes, ${total.toFixed(1)}s total audio`);
  return result;
}

/**
 * One continuous read for the whole video → one track, uniform tempo/tone. Scene
 * durations are word-proportional shares of it (a single read holds a near-constant
 * words/sec, so the visuals track the voice). Returns null on failure so the caller
 * falls back to per-scene synthesis.
 */
async function narrateOneShot(
  m: DemoManifest,
  voice: string,
  audioDir: string,
  narrationPath: string,
  backend: string,
  locale: string,
  audioRel: string,
): Promise<NarrationManifest | null> {
  const spoken = m.narration.filter((s) => s.text?.trim());
  if (spoken.length === 0) return null;
  const liveModel = process.env.GEMINI_LIVE_MODEL || "gemini-3.1-flash-live-preview";
  const fullWav = path.join(audioDir, "_narration.wav");
  // One turn = one continuous read. Paragraph breaks → natural beats between scenes.
  const fullText = spoken.map((s) => s.text.trim()).join("\n\n");
  let durs: number[];
  try {
    durs = await geminiLiveNarrate(voice, [{ text: fullText, outWav: fullWav }], liveModel, locale);
  } catch (e) {
    console.warn(`    one-shot live read failed: ${(e as Error).message.slice(0, 120)}`);
    return null;
  }
  if (!durs.length || durs[0] <= 0) return null;
  // A single uniform speed adjustment (not per-scene) keeps the tempo consistent.
  const speed = narrationLiveSpeed();
  if (Math.abs(speed - 1) > 0.001) await applyTempo(fullWav, speed);
  const D = wavDurationSec(fullWav);
  const totalWords = spoken.reduce((sum, s) => sum + countWords(s.text), 0) || 1;
  const scenes: NarrationScene[] = m.narration.map((spec) => {
    const text = spec.text?.trim() ?? "";
    const words = countWords(text);
    return {
      id: spec.id,
      kind: spec.kind,
      text,
      caption: spec.caption?.trim() || captionFromText(spec.text ?? ""),
      artifact: spec.artifact,
      beat: spec.beat,
      audio: undefined, // the single fullAudio track plays for the whole video
      durationSec: text ? D * (words / totalWords) : 0,
      holdSec: text ? 0 : defaultHold(spec.kind), // no gaps — the one read already pauses
    };
  });
  const manifest: NarrationManifest = { id: m.id, backend, voice, scenes, fullAudio: `${audioRel}/_narration.wav` };
  if (!isDefaultLocale(locale)) manifest.locale = locale;
  fs.writeFileSync(narrationPath, JSON.stringify(manifest, null, 2));
  console.log(`  ✓ narrated ${m.id}${isDefaultLocale(locale) ? "" : ` [${locale}]`} (one-shot): ${D.toFixed(1)}s single track across ${scenes.length} scenes`);
  return manifest;
}

function defaultHold(kind: NarrationScene["kind"]): number {
  return kind === "title" || kind === "outro" ? 0.4 : 0.2;
}

function captionFromText(text: string): string {
  const first = text.trim().split(/(?<=[.!?])\s/)[0] || text.trim();
  return first.length > 90 ? first.slice(0, 87) + "…" : first;
}
