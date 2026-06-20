// Generate the narration for the lab sample video (web/static/samples) with the
// same Gemini TTS setup the demo videos use — model gemini-3.x-flash-tts-preview,
// voice Charon, temperature 0.4, and the harness's British-narrator style prompt.
// Run from the harness root: `tsx scripts/gen-sample-narration.ts`.
//   GEMINI_API_KEY comes from ~/.config/neokapi/harness.env (loadEnv).
//
// The HTTP call goes through `curl` rather than node's fetch: in some
// environments node/undici stalls on the audio response (UND_ERR_HEADERS_TIMEOUT)
// while curl returns the same payload in ~2s. The request body is identical to
// src/narrate/synth.ts's geminiTts.
import { execFileSync } from "node:child_process";
import { writeFileSync } from "node:fs";
import { loadEnv } from "../src/lib/env.ts";

// Mirrors geminiStyle(en) + PRONUNCIATION_HINTS in src/narrate/synth.ts.
const STYLE =
  "Read this aloud in a clear, warm, professional British-English documentary-narrator voice, " +
  "at a measured, unhurried, and consistent pace — keep the exact same tone, energy, and tempo " +
  "from the first word to the last. Leave a clear pause between paragraphs. " +
  'Always pronounce the product name "kapi" as KAH-pee — two syllables, stress on the first, ' +
  "the 'a' as in 'father' — never ka-PEE or kap-ee: ";

const SCRIPT =
  "Welcome to neokapi. We localise images, audio, and video. " +
  "On screen text and speech are both translated. One pipeline, every modality.";

const MODEL = process.env.GEMINI_TTS_MODEL || "gemini-3.1-flash-tts-preview";
const VOICE = process.env.GEMINI_TTS_VOICE || "Charon";

function main(): void {
  loadEnv();
  const key = process.env.GEMINI_API_KEY;
  if (!key) throw new Error("GEMINI_API_KEY not set (add it to ~/.config/neokapi/harness.env)");

  const body = JSON.stringify({
    contents: [{ parts: [{ text: STYLE + SCRIPT }] }],
    generationConfig: {
      responseModalities: ["AUDIO"],
      temperature: 0.4,
      speechConfig: { voiceConfig: { prebuiltVoiceConfig: { voiceName: VOICE } } },
    },
  });
  const url = `https://generativelanguage.googleapis.com/v1beta/models/${MODEL}:generateContent?key=${key}`;

  console.log(`synthesizing narration via Gemini TTS (${MODEL}, voice ${VOICE})…`);
  // The TTS endpoint is intermittent here — it usually returns in a few seconds
  // but sometimes hangs with no bytes. Fast-fail (-m 60) and retry.
  let raw = "";
  let lastErr: unknown;
  for (let attempt = 1; attempt <= 8; attempt++) {
    try {
      raw = execFileSync(
        "curl",
        ["-sS", "-m", "60", "-X", "POST", url, "-H", "Content-Type: application/json", "-d", body],
        { maxBuffer: 128 * 1024 * 1024 },
      ).toString();
      if (raw.includes('"inlineData"')) {
        lastErr = undefined;
        break;
      }
      lastErr = new Error(`no audio: ${raw.slice(0, 200)}`);
    } catch (e) {
      lastErr = e;
    }
    console.warn(`  attempt ${attempt} failed; retrying…`);
  }
  if (lastErr) throw lastErr;

  const data = JSON.parse(raw) as {
    candidates?: { content?: { parts?: { inlineData?: { mimeType?: string; data?: string } }[] } }[];
  };
  const part = data.candidates?.[0]?.content?.parts?.find((p) => p.inlineData?.data);
  if (!part?.inlineData?.data) throw new Error(`no audio in response: ${raw.slice(0, 300)}`);
  const rate = Number(/rate=(\d+)/.exec(part.inlineData.mimeType ?? "")?.[1] ?? 24000);

  const pcm = Buffer.from(part.inlineData.data, "base64");
  const pcmPath = "/tmp/multimodal-narration.pcm";
  writeFileSync(pcmPath, pcm);
  // Gemini returns raw signed 16-bit little-endian mono PCM → mp3.
  execFileSync(
    "ffmpeg",
    // prettier-ignore
    ["-y", "-f", "s16le", "-ar", String(rate), "-ac", "1", "-i", pcmPath,
     "-ar", "44100", "-ac", "2", "-b:a", "128k", "public/multimodal-narration.mp3"],
    { stdio: "inherit" },
  );
  console.log("wrote public/multimodal-narration.mp3");
}

main();
