import fs from "node:fs";
import path from "node:path";
import { Buffer } from "node:buffer";
import type { DemoManifest, NarrationManifest, NarrationScene, NarrationSpec } from "../types.ts";
import { ensureDir, publicDemoDir } from "../lib/paths.ts";
import { run } from "../lib/exec.ts";

/** Style directive prepended to every Gemini TTS request to get a clear English narrator. */
const GEMINI_STYLE =
  "Read this aloud in a clear, warm, professional British-English documentary-narrator voice, " +
  "at a measured, unhurried, and consistent pace — keep the exact same tone, energy, and tempo " +
  "from the first word to the last. Always pronounce the product name \"kapi\" as KAH-pee — two " +
  "syllables, stress on the first, the 'a' as in 'father' — never ka-PEE or kap-ee: ";

type Backend = "gemini" | "elevenlabs" | "openai" | "say";

function pickBackend(): { backend: Backend; voice: string } {
  const forced = (process.env.NARRATION_BACKEND || "").toLowerCase() as Backend;
  if (forced === "elevenlabs" || (!forced && process.env.ELEVENLABS_API_KEY)) {
    if (process.env.ELEVENLABS_API_KEY) {
      return { backend: "elevenlabs", voice: process.env.ELEVENLABS_VOICE_ID || "Rachel" };
    }
  }
  if (forced === "openai" || (!forced && process.env.OPENAI_API_KEY && !process.env.GEMINI_API_KEY)) {
    if (process.env.OPENAI_API_KEY) {
      return { backend: "openai", voice: process.env.OPENAI_TTS_VOICE || "onyx" };
    }
  }
  if ((forced === "gemini" || !forced) && process.env.GEMINI_API_KEY) {
    return { backend: "gemini", voice: process.env.GEMINI_TTS_VOICE || "Charon" };
  }
  if (forced === "gemini" && !process.env.GEMINI_API_KEY) {
    console.warn("  ! NARRATION_BACKEND=gemini but GEMINI_API_KEY unset — falling back to macOS say");
  }
  return { backend: "say", voice: process.env.SAY_VOICE || "Daniel" };
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

/** Narration playback speed (atempo), pitch-preserving. Default 1.3 = noticeably brisker.
 *  This is the overall briskness target; per-scene pace is equalized around it (see
 *  narrateDemo) so scenes don't drift faster/slower than one another. */
function narrationSpeed(): number {
  const s = Number(process.env.NARRATION_SPEED);
  return Number.isFinite(s) && s > 0 ? s : 1.3;
}

/** TTS sampling temperature. Lower = steadier tone/pace across the separate per-scene
 *  calls. Default 0.4 ("stable" per Gemini's docs). Override with NARRATION_TEMPERATURE. */
function geminiTemperature(): number {
  const t = Number(process.env.NARRATION_TEMPERATURE);
  return Number.isFinite(t) && t >= 0 ? t : 0.4;
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

async function geminiTts(text: string, voice: string, outWav: string): Promise<void> {
  const key = process.env.GEMINI_API_KEY!;
  const model = process.env.GEMINI_TTS_MODEL || "gemini-2.5-flash-preview-tts";
  const url = `https://generativelanguage.googleapis.com/v1beta/models/${model}:generateContent?key=${key}`;
  const body = {
    contents: [{ parts: [{ text: GEMINI_STYLE + text }] }],
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
  const aiff = outWav + ".aiff";
  const r = await run("say", ["-v", voice, "-o", aiff, "--data-format=LEI16@24000", text], { timeoutMs: 60_000 });
  if (r.code !== 0) throw new Error(`say failed: ${r.stderr.slice(-300)}`);
  await toWav(aiff, outWav);
  fs.rmSync(aiff, { force: true });
}

async function synthOne(backend: Backend, voice: string, text: string, outWav: string): Promise<void> {
  // Pronunciation of "kapi" is handled by the style prompt (GEMINI_STYLE), not by
  // respelling the word — respelling it ("kah-pee") made the voice say "ka-PEE".
  switch (backend) {
    case "gemini":
      return geminiTts(text, voice, outWav);
    case "elevenlabs":
      return elevenLabsTts(text, voice, outWav);
    case "openai":
      return openaiTts(text, voice, outWav);
    case "say":
      return sayTts(text, voice, outWav);
  }
}

// ── Orchestration ─────────────────────────────────────────────────────────────

export interface NarrateOptions {
  force?: boolean;
}

/** Synthesize narration for every scene → public/<id>/audio/*.wav + narration.json. */
export async function narrateDemo(m: DemoManifest, opts: NarrateOptions = {}): Promise<NarrationManifest> {
  const pub = publicDemoDir(m.id);
  const audioDir = ensureDir(path.join(pub, "audio"));
  const narrationPath = path.join(pub, "narration.json");

  if (!opts.force && fs.existsSync(narrationPath)) {
    console.log(`  · narration exists for ${m.id} (use --force to re-run)`);
    return JSON.parse(fs.readFileSync(narrationPath, "utf8"));
  }

  const { backend, voice } = pickBackend();
  console.log(`  · narrating ${m.id} with ${backend} (${voice}), ${m.narration.length} scenes`);

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
      let attempt = 0;
      // Retry transient TTS errors a couple of times.
      while (true) {
        try {
          await synthOne(backend, voice, text, outWav);
          break;
        } catch (e) {
          attempt++;
          if (attempt >= 3) throw e;
          console.warn(`    retry ${attempt} (${(e as Error).message.slice(0, 120)})`);
          await new Promise((r) => setTimeout(r, 1500 * attempt));
        }
      }
      synthed.push({ spec, wavName, outWav, spoken: true, words: countWords(text), naturalDur: wavDurationSec(outWav) });
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
  const minF = speed * 0.8;
  const maxF = speed * 1.25;

  const scenes: NarrationScene[] = [];
  for (const s of synthed) {
    let durationSec = 0;
    let audio: string | undefined;
    if (s.spoken) {
      const wps = s.naturalDur > 0 ? s.words / s.naturalDur : 0;
      const factor = wps > 0 && targetWps > 0 ? Math.min(maxF, Math.max(minF, targetWps / wps)) : speed;
      await applyTempo(s.outWav, factor);
      durationSec = wavDurationSec(s.outWav);
      audio = `audio/${s.wavName}`;
    }
    scenes.push({
      id: s.spec.id,
      kind: s.spec.kind,
      text: s.spec.text.trim(),
      caption: s.spec.caption?.trim() || captionFromText(s.spec.text),
      artifact: s.spec.artifact,
      audio,
      durationSec,
      holdSec: s.spec.holdSec ?? defaultHold(s.spec.kind),
    });
  }

  const manifest: NarrationManifest = { id: m.id, backend, voice, scenes };
  fs.writeFileSync(narrationPath, JSON.stringify(manifest, null, 2));
  const total = scenes.reduce((s, sc) => s + sc.durationSec + sc.holdSec, 0);
  console.log(`  ✓ narrated ${m.id}: ${scenes.length} scenes, ${total.toFixed(1)}s total audio`);
  return manifest;
}

function defaultHold(kind: NarrationScene["kind"]): number {
  return kind === "title" || kind === "outro" ? 0.4 : 0.2;
}

function captionFromText(text: string): string {
  const first = text.trim().split(/(?<=[.!?])\s/)[0] || text.trim();
  return first.length > 90 ? first.slice(0, 87) + "…" : first;
}
