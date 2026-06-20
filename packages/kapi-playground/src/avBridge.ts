// avBridge — video demux in the browser via ffmpeg.wasm (@ffmpeg/ffmpeg), the
// same ffmpeg the native kapi-av plugin bundles. It splits a video into the two
// streams a localization flow extracts from (AD-030): the audio track (16 kHz
// mono WAV → speech recognition via asrBridge) and sampled frames (PNG → OCR via
// visionBridge). Only the runtime differs: WebAssembly here, native there.
//
// The single-threaded @ffmpeg/core is used so no SharedArrayBuffer (and thus no
// COOP/COEP cross-origin-isolation headers) is required — it runs on a plain
// static host like GitHub Pages. The ~32 MB core loads on first use.

import { FFmpeg } from "@ffmpeg/ffmpeg";
import { toBlobURL } from "@ffmpeg/util";

export interface AVFrame {
  /** Frame time from the start of the video, in milliseconds. */
  timeMs: number;
  /** A blob: URL for the frame PNG (revoke with revokeFrames when done). */
  url: string;
}

export interface DemuxResult {
  /** 16 kHz mono WAV bytes of the audio track (decode via asrBridge.decodeAudio). */
  audio: Uint8Array;
  /** Sampled, time-stamped frames. */
  frames: AVFrame[];
}

export interface DemuxOptions {
  /** Frame sampling rate (frames per second). Default 1. */
  fps?: number;
  /** Core/inference progress in [0,1]. */
  onProgress?: (frac: number) => void;
}

const CORE_VERSION = "0.12.10";
const CORE_BASE = `https://unpkg.com/@ffmpeg/core@${CORE_VERSION}/dist/umd`;

let ffmpeg: Promise<FFmpeg> | null = null;

/** Whether the ffmpeg core has started loading (so the UI can show a one-time hint). */
export function ffmpegLoaded(): boolean {
  return ffmpeg !== null;
}

async function ensureFFmpeg(onProgress?: (frac: number) => void): Promise<FFmpeg> {
  if (!ffmpeg) {
    ffmpeg = (async () => {
      const f = new FFmpeg();
      if (onProgress) f.on("progress", (e: { progress: number }) => onProgress(e.progress));
      await f.load({
        coreURL: await toBlobURL(`${CORE_BASE}/ffmpeg-core.js`, "text/javascript"),
        wasmURL: await toBlobURL(`${CORE_BASE}/ffmpeg-core.wasm`, "application/wasm"),
      });
      return f;
    })();
  }
  return ffmpeg;
}

function asBytes(data: Uint8Array | string): Uint8Array {
  return typeof data === "string" ? new TextEncoder().encode(data) : data;
}

/**
 * demux extracts the audio track (16 kHz mono WAV) and samples frames (PNG) from
 * a video, mirroring `core/av.Demux`. Pass the video file's bytes; get the audio
 * bytes (feed to asrBridge.decodeAudio + transcribe) and time-stamped frame URLs
 * (feed to visionBridge.ocr). Revoke the frame URLs with revokeFrames when done.
 */
export async function demux(input: Uint8Array, opts: DemuxOptions = {}): Promise<DemuxResult> {
  const fps = opts.fps ?? 1;
  const f = await ensureFFmpeg(opts.onProgress);

  await f.writeFile("input", input);

  // Audio: downmix to 16 kHz mono WAV (Whisper's input rate).
  await f.exec(["-i", "input", "-vn", "-ac", "1", "-ar", "16000", "-f", "wav", "audio.wav"]);
  const audio = asBytes(await f.readFile("audio.wav"));

  // Frames: sample at `fps` to numbered PNGs.
  await f.exec(["-i", "input", "-vf", `fps=${fps}`, "frame_%04d.png"]);
  const entries = await f.listDir("/");
  const frames: AVFrame[] = [];
  for (const e of entries) {
    const m = /^frame_(\d+)\.png$/.exec(e.name);
    if (!m) continue;
    const idx = parseInt(m[1], 10);
    const png = asBytes(await f.readFile(e.name));
    const blob = new Blob([png as BlobPart], { type: "image/png" });
    frames.push({ timeMs: Math.round(((idx - 1) / fps) * 1000), url: URL.createObjectURL(blob) });
    await f.deleteFile(e.name);
  }
  frames.sort((a, b) => a.timeMs - b.timeMs);

  await f.deleteFile("input");
  await f.deleteFile("audio.wav");

  return { audio, frames };
}

/** Revoke the blob: URLs returned by demux (call when the frames are no longer shown). */
export function revokeFrames(frames: AVFrame[]): void {
  for (const fr of frames) URL.revokeObjectURL(fr.url);
}
