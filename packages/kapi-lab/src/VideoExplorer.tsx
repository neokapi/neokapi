import React, { useCallback, useRef, useState } from "react";
import { VideoPlayer, type ContentNode, type ContentTree } from "@neokapi/ui-primitives/preview";
import { decodeAudio, transcribe, type ASRSegment } from "@neokapi/kapi-playground/asrBridge";
import { demux, revokeFrames, type AVFrame } from "@neokapi/kapi-playground/avBridge";
import { ocr, type OCRLine } from "@neokapi/kapi-playground/visionBridge";
import { ensurePlugin } from "@neokapi/kapi-playground/plugins";

// VideoExplorer — drop in a video and the whole multimodal pipeline runs in your
// browser: ffmpeg.wasm (@ffmpeg/ffmpeg) demuxes it into a 16 kHz audio track and
// sampled frames; Whisper (asrBridge) transcribes the speech into subtitle cues;
// PP-OCRv5 (visionBridge) reads the on-screen frame text — the same engines the
// native kapi-av / kapi-asr / kapi-vision plugins run, only the runtime differs
// (WebAssembly here, native there). The result is a VideoPlayer with a synced
// subtitle track plus frame-OCR boxes overlaid at their timecode. Nothing is
// mocked. The ffmpeg core (~32 MB) and the Whisper model (~40 MB) load on first
// use; on-screen-text OCR needs the vision models (served by the host page).

export interface VideoSampleSpec {
  url: string;
  name: string;
}

export interface VideoExplorerProps {
  samples?: VideoSampleSpec[];
  /** Base URL the OCR models are served from (same-origin/CORS); enables frame OCR. */
  modelBase?: string;
}

interface FrameOCR {
  timeMs: number;
  lines: OCRLine[];
}

async function frameRaster(
  url: string,
): Promise<{ data: Uint8ClampedArray; width: number; height: number }> {
  const img = new Image();
  img.src = url;
  await img.decode();
  const canvas = document.createElement("canvas");
  canvas.width = img.naturalWidth;
  canvas.height = img.naturalHeight;
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new Error("2d canvas unavailable");
  ctx.drawImage(img, 0, 0);
  const id = ctx.getImageData(0, 0, canvas.width, canvas.height);
  return { data: id.data, width: canvas.width, height: canvas.height };
}

function buildTree(speech: ASRSegment[], frames: FrameOCR[]): ContentTree {
  const blocks: ContentNode[] = [];
  speech.forEach((s, i) =>
    blocks.push({
      kind: "block",
      id: `s${i}`,
      type: "subtitle",
      source: [{ text: s.text }],
      timing: { startMs: s.startMs, endMs: s.endMs },
    }),
  );
  let fi = 0;
  for (const fr of frames) {
    for (const l of fr.lines) {
      blocks.push({
        kind: "block",
        id: `f${fi++}`,
        type: "line",
        source: [{ text: l.text }],
        // Absolute-pixel geometry (resolution 0) — coords are the frame's pixels,
        // which map onto the video's natural size in the player's overlay.
        geometry: { x: l.x, y: l.y, w: l.w, h: l.h, resolution: 0 },
        timing: { startMs: fr.timeMs, endMs: fr.timeMs + 1000 },
        structure: { role: "paragraph" },
      });
    }
  }
  return {
    format: "video",
    root: [{ kind: "layer", id: "doc", name: "video", children: blocks }],
    stats: { layers: 1, groups: 0, blocks: blocks.length, data: 0, media: 0, runs: blocks.length },
  };
}

export default function VideoExplorer({
  samples = [],
  modelBase,
}: VideoExplorerProps): React.ReactElement {
  const [src, setSrc] = useState<string | null>(samples[0]?.url ?? null);
  const [tree, setTree] = useState<ContentTree | null>(null);
  const [stage, setStage] = useState<"" | "demux" | "asr" | "ocr">("");
  const [progress, setProgress] = useState(0);
  const [doOCR, setDoOCR] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const fileRef = useRef<HTMLInputElement | null>(null);
  const framesRef = useRef<AVFrame[]>([]);

  const load = useCallback((url: string) => {
    setSrc(url);
    setTree(null);
    setError(null);
  }, []);

  const run = useCallback(async () => {
    if (!src) return;
    setError(null);
    try {
      setStage("demux");
      setProgress(0);
      // Route each model/core download through the plugin manager so the navbar
      // status widget reflects av/asr/vision loading; the bridges reuse them.
      await ensurePlugin("av");
      const bytes = new Uint8Array(await (await fetch(src)).arrayBuffer());
      const { audio, frames } = await demux(bytes, { fps: 1, onProgress: setProgress });
      revokeFrames(framesRef.current);
      framesRef.current = frames;

      setStage("asr");
      await ensurePlugin("asr");
      const audioBuf = await decodeAudio(audio.buffer.slice(0) as ArrayBuffer);
      const asr = await transcribe(audioBuf, { onProgress: setProgress });

      const frameOCR: FrameOCR[] = [];
      if (doOCR && modelBase) {
        setStage("ocr");
        await ensurePlugin("vision");
        for (const fr of frames) {
          const raster = await frameRaster(fr.url);
          const res = await ocr(raster, modelBase);
          if (res.lines.length > 0) frameOCR.push({ timeMs: fr.timeMs, lines: res.lines });
        }
      }
      setTree(buildTree(asr.segments, frameOCR));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setStage("");
    }
  }, [src, doOCR, modelBase]);

  const stageLabel =
    stage === "demux"
      ? "Demuxing with ffmpeg.wasm…"
      : stage === "asr"
        ? "Transcribing speech (Whisper)…"
        : stage === "ocr"
          ? "Reading on-screen text (OCR)…"
          : "";

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        {samples.map((s) => (
          <button
            key={s.url}
            type="button"
            onClick={() => load(s.url)}
            className="rounded-md border px-3 py-1.5 text-sm hover:bg-muted/60"
          >
            {s.name}
          </button>
        ))}
        <button
          type="button"
          onClick={() => fileRef.current?.click()}
          className="rounded-md border px-3 py-1.5 text-sm hover:bg-muted/60"
        >
          Upload video…
        </button>
        <input
          ref={fileRef}
          type="file"
          accept="video/*"
          className="hidden"
          onChange={(e) => {
            const f = e.target.files?.[0];
            if (f) load(URL.createObjectURL(f));
          }}
        />
        {modelBase && (
          <label className="flex items-center gap-1.5 text-sm text-muted-foreground">
            <input type="checkbox" checked={doOCR} onChange={(e) => setDoOCR(e.target.checked)} />
            OCR on-screen text
          </label>
        )}
        <button
          type="button"
          onClick={run}
          disabled={!src || stage !== ""}
          className="rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground disabled:opacity-50"
        >
          {stage !== "" ? "Processing…" : "Localize"}
        </button>
      </div>

      {src && !tree && (
        <video src={src} controls className="w-full max-w-xl rounded-md">
          <track kind="captions" />
        </video>
      )}

      {stage !== "" && (
        <div className="text-sm text-muted-foreground">
          {stageLabel} {progress > 0 ? `${Math.round(progress * 100)}%` : ""}
        </div>
      )}
      {error && <div className="text-sm text-destructive">{error}</div>}

      {tree && src && <VideoPlayer src={src} tree={tree} showFrameOCR className="max-w-2xl" />}
    </div>
  );
}
