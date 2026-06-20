import React, { useEffect, useMemo, useRef, useState } from "react";
import {
  MediaCanvas,
  SubtitleTimeline,
  VideoPlayer,
  type ContentNode,
  type ContentTree,
} from "@neokapi/ui-primitives/preview";

// MultimodalShowcase — a pre-recorded (canned-data) walkthrough of the multimodal
// localization story: image OCR, audio subtitles, and video, each translated to
// French. It runs no engine — the extraction results are baked into ContentTree
// fixtures (mirroring `kapi inspect` output), and a simulated playhead advances
// through the timed chapters so the subtitle highlight and frame-OCR overlay
// animate exactly as they would over real playback. This makes the story work
// anywhere, instantly, with no model download or ffmpeg — the reliable companion
// to the live in-browser labs (/lab/audio, /lab/video) and /lab/vision.

function tree(format: string, root: ContentNode[]): ContentTree {
  return { format, root, stats: { layers: 0, groups: 0, blocks: 0, data: 0, media: 0, runs: 0 } };
}

function cue(id: string, startMs: number, endMs: number, src: string, tgt: string): ContentNode {
  return {
    kind: "block",
    id,
    type: "subtitle",
    source: [{ text: src }],
    targets: { "fr-FR": [{ text: tgt }] },
    timing: { startMs, endMs },
  };
}

function ocrLine(
  id: string,
  x: number,
  y: number,
  w: number,
  h: number,
  src: string,
  tgt: string,
  role: string,
  timing?: { startMs: number; endMs: number },
): ContentNode {
  return {
    kind: "block",
    id,
    type: "line",
    source: [{ text: src }],
    targets: { "fr-FR": [{ text: tgt }] },
    geometry: { x, y, w, h, resolution: 100 },
    structure: { role },
    ...(timing ? { timing } : {}),
  };
}

// A small inline SVG poster (base64 so every browser renders it).
function svgPoster(title: string, subtitle: string): string {
  const svg =
    "<svg xmlns='http://www.w3.org/2000/svg' width='480' height='270'>" +
    "<rect width='480' height='270' fill='#0f172a'/>" +
    `<text x='28' y='80' fill='#f8fafc' font-family='sans-serif' font-size='34'>${title}</text>` +
    `<text x='28' y='150' fill='#93c5fd' font-family='sans-serif' font-size='22'>${subtitle}</text>` +
    "</svg>";
  return "data:image/svg+xml;base64," + btoa(svg);
}

const IMAGE_POSTER = svgPoster("Invoice", "Total: $42.00");
const VIDEO_POSTER = svgPoster("Welcome", "On-screen: Chapter 1");

const imageTree = tree("image", [
  {
    kind: "media",
    id: "img",
    media: { mimeType: "image/svg+xml", filename: "invoice.svg", uri: IMAGE_POSTER },
  },
  ocrLine("i1", 5.5, 22, 22, 13, "Invoice", "Facture", "heading"),
  ocrLine("i2", 5.5, 50, 38, 9, "Total: $42.00", "Total : 42,00 $", "paragraph"),
]);

const audioTree = tree("audio", [
  cue("a1", 0, 2200, "Welcome to the show.", "Bienvenue à l'émission."),
  cue(
    "a2",
    2200,
    5200,
    "Today we explore localization.",
    "Aujourd'hui, nous explorons la localisation.",
  ),
  cue("a3", 5200, 8000, "Let's begin.", "Commençons."),
]);

const videoTree = tree("video", [
  // Speech track (timing only) — becomes the subtitle track.
  cue(
    "v1",
    0,
    2600,
    "In this clip we localize a video.",
    "Dans ce clip, nous localisons une vidéo.",
  ),
  cue(
    "v2",
    2600,
    6000,
    "Both speech and on-screen text are translated.",
    "La parole et le texte à l'écran sont traduits.",
  ),
  // On-screen frame text (timing + geometry) — overlaid at its timecode.
  ocrLine("vf1", 6, 18, 26, 12, "Chapter 1", "Chapitre 1", "heading", { startMs: 0, endMs: 3000 }),
  ocrLine("vf2", 6, 18, 30, 12, "Summary", "Résumé", "heading", { startMs: 3000, endMs: 6000 }),
]);

interface Chapter {
  id: string;
  title: string;
  blurb: string;
  durationMs: number;
  render: (playheadMs: number) => React.ReactElement;
}

const CHAPTERS: Chapter[] = [
  {
    id: "image",
    title: "Image — OCR + translate",
    blurb:
      "kapi-vision reads the text in an image as geometry-anchored blocks; translating them round-trips a localized alt-text (and the whole image is replaceable per locale).",
    durationMs: 0,
    render: () => (
      <MediaCanvas src={IMAGE_POSTER} tree={imageTree} alt="Invoice" className="w-full" />
    ),
  },
  {
    id: "audio",
    title: "Audio — speech → subtitles",
    blurb:
      "kapi-asr transcribes speech into timing-anchored cues; translate fills the target, and the audio-to-subtitles flow writes a translated .vtt/.srt.",
    durationMs: 8000,
    render: (ms) => <SubtitleTimeline tree={audioTree} currentTimeMs={ms} className="w-full" />,
  },
  {
    id: "video",
    title: "Video — speech + on-screen text",
    blurb:
      "kapi-av demuxes the video; the speech track becomes the subtitle track (frame OCR is filtered out of it), while on-screen frame text is overlaid at its timecode.",
    durationMs: 6000,
    render: (ms) => (
      <VideoPlayer
        src={VIDEO_POSTER}
        poster={VIDEO_POSTER}
        tree={videoTree}
        currentTimeMs={ms}
        showFrameOCR
        // Canned showcase: the playhead is the chapter's own ▶/⏸ (currentTimeMs),
        // not a real video — hide the dead native controls. The live /lab/video
        // lab plays a real uploaded video.
        controls={false}
        className="w-full"
      />
    ),
  },
];

export interface MultimodalShowcaseProps {
  /** Chapter to start on (0-based). */
  initialChapter?: number;
  className?: string;
}

export default function MultimodalShowcase({
  initialChapter = 0,
  className,
}: MultimodalShowcaseProps): React.ReactElement {
  const [chapterIdx, setChapterIdx] = useState(
    Math.min(Math.max(initialChapter, 0), CHAPTERS.length - 1),
  );
  const [playheadMs, setPlayheadMs] = useState(0);
  const [playing, setPlaying] = useState(true);
  const chapter = CHAPTERS[chapterIdx];
  const timed = chapter.durationMs > 0;

  // Reset the playhead whenever the chapter changes.
  useEffect(() => {
    setPlayheadMs(0);
  }, [chapterIdx]);

  // Simulated playback: advance the playhead for timed chapters, looping.
  const rafRef = useRef<number | null>(null);
  useEffect(() => {
    if (!playing || !timed) return;
    const step = 80;
    const id = window.setInterval(() => {
      setPlayheadMs((ms) => (ms + step > chapter.durationMs ? 0 : ms + step));
    }, step);
    rafRef.current = id as unknown as number;
    return () => window.clearInterval(id);
  }, [playing, timed, chapter.durationMs]);

  const go = (delta: number) => {
    setChapterIdx((i) => Math.min(Math.max(i + delta, 0), CHAPTERS.length - 1));
  };

  const tabs = useMemo(
    () =>
      CHAPTERS.map((c, i) => (
        <button
          key={c.id}
          type="button"
          onClick={() => setChapterIdx(i)}
          aria-current={i === chapterIdx ? "true" : undefined}
          className={
            "rounded-md px-3 py-1.5 text-sm font-medium transition-colors " +
            (i === chapterIdx
              ? "bg-primary text-primary-foreground"
              : "bg-muted text-muted-foreground hover:bg-muted/70")
          }
        >
          {i + 1}. {c.title.split(" — ")[0]}
        </button>
      )),
    [chapterIdx],
  );

  return (
    <div className={"flex flex-col gap-4 " + (className ?? "")} data-testid="multimodal-showcase">
      <div className="flex flex-wrap gap-2">{tabs}</div>

      <div className="rounded-lg border bg-card p-4">
        <h3 className="mb-1 text-base font-semibold">{chapter.title}</h3>
        <p className="mb-4 text-sm text-muted-foreground">{chapter.blurb}</p>
        <div key={chapter.id}>{chapter.render(playheadMs)}</div>
      </div>

      <div className="flex items-center justify-between">
        <div className="flex gap-2">
          <button
            type="button"
            onClick={() => go(-1)}
            disabled={chapterIdx === 0}
            className="rounded-md border px-3 py-1.5 text-sm disabled:opacity-40"
          >
            ← Prev
          </button>
          <button
            type="button"
            onClick={() => go(1)}
            disabled={chapterIdx === CHAPTERS.length - 1}
            className="rounded-md border px-3 py-1.5 text-sm disabled:opacity-40"
          >
            Next →
          </button>
        </div>
        {timed && (
          <button
            type="button"
            onClick={() => setPlaying((p) => !p)}
            className="rounded-md border px-3 py-1.5 text-sm"
          >
            {playing ? "⏸ Pause" : "▶ Play"}
          </button>
        )}
      </div>
    </div>
  );
}
