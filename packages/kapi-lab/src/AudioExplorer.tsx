import React, { useCallback, useRef, useState } from "react";
import { AudioPlayer, type ContentNode, type ContentTree } from "@neokapi/ui-primitives/preview";
import { decodeAudio, transcribe, type ASRSegment } from "@neokapi/kapi-playground/asrBridge";
import { ensurePlugin } from "@neokapi/kapi-playground/plugins";

// AudioExplorer — drop in (or pick) an audio file and run real Whisper speech
// recognition in your browser via @huggingface/transformers (onnxruntime-web),
// the same Whisper family the native kapi-asr plugin runs through whisper.cpp.
// The recognized segments become timing-anchored cues rendered in the AudioPlayer
// (play it and the active cue highlights), exactly the shape the audio-to-subtitles
// flow translates and writes to .vtt/.srt. Nothing is mocked — only the runtime
// differs (WebAssembly here, native there). The model (~40 MB) loads on first use.

export interface AudioSampleSpec {
  url: string;
  name: string;
}

export interface AudioExplorerProps {
  samples?: AudioSampleSpec[];
}

function treeFromSegments(segs: ASRSegment[]): ContentTree {
  const blocks: ContentNode[] = segs.map((s, i) => ({
    kind: "block",
    id: `c${i}`,
    type: "subtitle",
    source: [{ text: s.text }],
    timing: { startMs: s.startMs, endMs: s.endMs },
  }));
  return {
    format: "audio",
    root: [{ kind: "layer", id: "doc", name: "audio", children: blocks }],
    stats: { layers: 1, groups: 0, blocks: blocks.length, data: 0, media: 0, runs: blocks.length },
  };
}

export default function AudioExplorer({ samples = [] }: AudioExplorerProps): React.ReactElement {
  const [src, setSrc] = useState<string | null>(samples[0]?.url ?? null);
  const [tree, setTree] = useState<ContentTree | null>(null);
  const [busy, setBusy] = useState(false);
  const [progress, setProgress] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const fileRef = useRef<HTMLInputElement | null>(null);

  const load = useCallback((url: string) => {
    setSrc(url);
    setTree(null);
    setError(null);
  }, []);

  const onPick = useCallback(
    (file: File) => {
      load(URL.createObjectURL(file));
    },
    [load],
  );

  const run = useCallback(async () => {
    if (!src) return;
    setBusy(true);
    setError(null);
    setProgress(0);
    try {
      // Route the model download through the plugin manager so the navbar
      // status widget reflects the `asr` plugin loading; transcribe() then
      // reuses the warmed model.
      await ensurePlugin("asr");
      const bytes = await (await fetch(src)).arrayBuffer();
      const audio = await decodeAudio(bytes);
      const res = await transcribe(audio, { onProgress: setProgress });
      setTree(treeFromSegments(res.segments));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }, [src]);

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
          Upload audio…
        </button>
        <input
          ref={fileRef}
          type="file"
          accept="audio/*,video/*"
          className="hidden"
          onChange={(e) => {
            const f = e.target.files?.[0];
            if (f) onPick(f);
          }}
        />
        <button
          type="button"
          onClick={run}
          disabled={!src || busy}
          className="rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground disabled:opacity-50"
        >
          {busy ? "Transcribing…" : "Transcribe"}
        </button>
      </div>

      {src && !tree && (
        <audio src={src} controls className="w-full max-w-xl">
          <track kind="captions" />
        </audio>
      )}

      {busy && (
        <div className="text-sm text-muted-foreground">
          Running Whisper in your browser… {progress > 0 ? `${Math.round(progress * 100)}%` : ""}
        </div>
      )}
      {error && <div className="text-sm text-destructive">{error}</div>}

      {tree && src && <AudioPlayer src={src} tree={tree} className="max-w-2xl" />}
    </div>
  );
}
