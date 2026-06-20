import React from "react";
import { AbsoluteFill, Audio, Sequence, interpolate, staticFile, useCurrentFrame } from "remotion";

// MultimodalSample — a short (~10s) clip for the in-browser audio/video labs.
// Four high-contrast title cards (clean on-screen text for OCR) over a narrated
// voice track (`say` → mp3, for speech recognition), so a visitor can try the
// labs without supplying their own file: ffmpeg.wasm demuxes it, Whisper
// transcribes the narration, and PP-OCRv5 reads the on-screen text.

export const SAMPLE_FPS = 30;
export const SAMPLE_WIDTH = 1280;
export const SAMPLE_HEIGHT = 720;
const PER = 109; // frames per slide (~3.6s); 4 slides ≈ 14.5s, covering the narration.
export const SAMPLE_FRAMES = 4 * PER;

const SLIDES: { title: string; sub: string }[] = [
  { title: "Welcome to neokapi", sub: "AI-native localization" },
  { title: "Images · Audio · Video", sub: "localize every modality" },
  { title: "Speech & on-screen text", sub: "both translated" },
  { title: "One pipeline", sub: "every modality" },
];

const Slide: React.FC<{ title: string; sub: string }> = ({ title, sub }) => {
  const frame = useCurrentFrame();
  const opacity = interpolate(frame, [0, 8, PER - 8, PER], [0, 1, 1, 0], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  return (
    <AbsoluteFill style={{ justifyContent: "center", alignItems: "center", opacity }}>
      <div
        style={{
          color: "#f8fafc",
          fontSize: 84,
          fontWeight: 800,
          fontFamily: "Helvetica, Arial, sans-serif",
          textAlign: "center",
          padding: "0 64px",
          letterSpacing: -1,
        }}
      >
        {title}
      </div>
      <div
        style={{
          color: "#93c5fd",
          fontSize: 40,
          marginTop: 24,
          fontFamily: "Helvetica, Arial, sans-serif",
        }}
      >
        {sub}
      </div>
    </AbsoluteFill>
  );
};

export const MultimodalSample: React.FC = () => (
  <AbsoluteFill style={{ backgroundColor: "#0f172a" }}>
    <Audio src={staticFile("multimodal-narration.mp3")} />
    {SLIDES.map((s, i) => (
      <Sequence key={s.title} from={i * PER} durationInFrames={PER}>
        <Slide title={s.title} sub={s.sub} />
      </Sequence>
    ))}
  </AbsoluteFill>
);
